package start

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jenkins-x/jx-api/pkg/client/clientset/versioned"
	"github.com/jenkins-x/jx-helpers/pkg/input"
	"github.com/jenkins-x/jx-helpers/pkg/input/inputfactory"
	"github.com/jenkins-x/jx-helpers/pkg/kube"
	"github.com/jenkins-x/jx-helpers/pkg/kube/jxclient"
	"github.com/jenkins-x/jx-helpers/pkg/options"
	"github.com/jenkins-x/jx-helpers/pkg/stringhelpers"
	"github.com/jenkins-x/jx-pipeline/pkg/constants"
	"github.com/jenkins-x/jx-pipeline/pkg/sourcerepos"
	"github.com/jenkins-x/jx-pipeline/pkg/tektonlog"
	"github.com/jenkins-x/jx-pipeline/pkg/triggers"
	"github.com/jenkins-x/jx/v2/pkg/tekton/metapipeline"
	"github.com/jenkins-x/lighthouse/pkg/config"
	"github.com/pkg/errors"
	"k8s.io/client-go/kubernetes"

	"github.com/jenkins-x/jx-helpers/pkg/cobras/helper"

	"github.com/spf13/cobra"

	"github.com/jenkins-x/jx-helpers/pkg/cobras/templates"
	"github.com/jenkins-x/jx-logging/pkg/log"
)

const (
	releaseBranchName = "master"
)

// Options contains the command line options
type Options struct {
	options.BaseOptions

	Args                []string
	Output              string
	Filter              string
	Branch              string
	PipelineKind        string
	LighthouseConfigMap string
	ServiceAccount      string
	Namespace           string
	Wait                bool
	Tail                bool
	WaitDuration        time.Duration
	PollPeriod          time.Duration
	KubeClient          kubernetes.Interface
	JXClient            versioned.Interface
	Input               input.Interface

	// meta pipeline options
	Context      string
	CustomLabels []string
	CustomEnvs   []string
}

var (
	startPipelineLong = templates.LongDesc(`
		Starts the pipeline build.

`)

	startPipelineExample = templates.Examples(`
		# Start a pipeline
		jx pipeline start foo

		# Select the pipeline to start
		jx pipeline start

		# Select the pipeline to start and tail the log
		jx pipeline start -t
	`)
)

// NewCmdPipelineStart creates the command
func NewCmdPipelineStart() (*cobra.Command, *Options) {
	o := &Options{}

	cmd := &cobra.Command{
		Use:     "start",
		Short:   "Starts one or more pipelines",
		Long:    startPipelineLong,
		Example: startPipelineExample,
		Aliases: []string{"build", "run"},
		Run: func(cmd *cobra.Command, args []string) {
			o.Args = args
			err := o.Run()
			helper.CheckErr(err)
		},
	}
	cmd.Flags().BoolVarP(&o.Tail, "tail", "t", false, "Tails the build log to the current terminal")
	cmd.Flags().StringVarP(&o.Filter, "filter", "f", "", "Filters all the available jobs by those that contain the given text")
	cmd.Flags().StringVarP(&o.Context, "context", "c", "", "An optional Prow pipeline context")
	cmd.Flags().StringVarP(&o.Branch, "branch", "", "", "The branch to start. If not specified defaults to master")
	cmd.Flags().StringVarP(&o.PipelineKind, "kind", "", "", "The kind of pipeline such as release or pullrequest")
	cmd.Flags().StringVar(&o.ServiceAccount, "service-account", tektonlog.DefaultPipelineSA, "The Kubernetes ServiceAccount to use to run the meta pipeline")
	cmd.Flags().StringVarP(&o.LighthouseConfigMap, "configmap", "", constants.LighthouseConfigMapName, "The name of the Lighthouse ConfigMap to find the trigger configurations")
	cmd.Flags().StringArrayVarP(&o.CustomLabels, "label", "l", nil, "List of custom labels to be applied to the generated PipelineRun (can be use multiple times)")
	cmd.Flags().StringArrayVarP(&o.CustomEnvs, "env", "e", nil, "List of custom environment variables to be applied to the generated PipelineRun that are created (can be use multiple times)")
	cmd.Flags().BoolVarP(&o.Wait, "wait", "", false, "Waits until the trigger has been setup in Lighthouse for when a new repository is being imported via GitOps")
	cmd.Flags().DurationVarP(&o.WaitDuration, "duration", "", time.Minute*20, "Maximum duration to wait for one or more matching triggers to be setup in Lighthouse. Useful for when a new repository is being imported via GitOps")
	cmd.Flags().DurationVarP(&o.PollPeriod, "poll-period", "", time.Second*2, "Poll period when waiting for one or more matching triggers to be setup in Lighthouse. Useful for when a new repository is being imported via GitOps")

	return cmd, o
}

// Validate verifies things are setup correctly
func (o *Options) Validate() error {
	var err error
	o.KubeClient, o.Namespace, err = kube.LazyCreateKubeClientAndNamespace(o.KubeClient, o.Namespace)
	if err != nil {
		return errors.Wrapf(err, "failed to create kube client")
	}
	o.JXClient, err = jxclient.LazyCreateJXClient(o.JXClient)
	if err != nil {
		return errors.Wrapf(err, "failed to create the jx client")
	}

	if o.Input == nil {
		o.Input = inputfactory.NewInput(&o.BaseOptions)
	}
	return nil
}

// Run implements this command
func (o *Options) Run() error {
	err := o.Validate()
	if err != nil {
		return errors.Wrapf(err, "failed to validate options")
	}

	args := o.Args

	names, err := o.getFilteredTriggerNames(o.KubeClient, o.Namespace)
	if err != nil {
		return errors.Wrapf(err, "failed to get trigger names")
	}

	if len(args) == 0 {
		if len(names) == 0 {
			return errors.New("no jobs found to trigger")
		}
		sort.Strings(names)

		defaultName := ""
		for _, n := range names {
			if strings.HasSuffix(n, "/master") {
				defaultName = n
				break
			}
		}
		name := ""
		name, err = o.Input.PickNameWithDefault(names, "Which pipeline do you want to start: ", defaultName, "")
		if err != nil {
			return err
		}
		args = []string{name}
	}
	for _, a := range args {
		err = o.createMetaPipeline(a)
		if err != nil {
			return err
		}
	}
	return nil
}

func (o *Options) getFilteredTriggerNames(kubeClient kubernetes.Interface, ns string) ([]string, error) {
	end := time.Now().Add(o.WaitDuration)
	name := o.LighthouseConfigMap
	logWaiting := false

	for {
		cfg, err := triggers.LoadLighthouseConfig(kubeClient, ns, name, o.Wait)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to load lighthouse config")
		}
		names := o.pipelineNames(cfg)
		names = stringhelpers.StringsContaining(names, o.Filter)

		if len(names) > 0 || !o.Wait {
			return names, nil
		}

		if time.Now().After(end) {
			return nil, errors.Errorf("failed to find trigger in the lighthouse configuration in ConfigMap %s in namespace %s matching filter: '%s' within %s", name, ns, o.Filter, o.WaitDuration.String())
		}

		if !logWaiting {
			logWaiting = true
			log.Logger().Infof("waiting up to %s for a trigger to be added to the lighthouse configuration in ConfigMap %s in namespace %s matching filter: '%s'", o.WaitDuration.String(), name, ns, o.Filter)
		}
		time.Sleep(o.PollPeriod)
	}
}

func (o *Options) createMetaPipeline(jobName string) error {
	parts := strings.Split(jobName, "/")
	if len(parts) < 2 {
		return fmt.Errorf("job name [%s] does not match org/repo/branch format", jobName)
	}
	owner := parts[0]
	repo := parts[1]
	branch := ""
	if len(parts) > 2 {
		branch = parts[2]
	}
	if o.Branch != "" {
		branch = o.Branch
	}
	if branch == "" {
		branch = "master"
	}

	var err error
	jxClient := o.JXClient
	ns := o.Namespace

	sr, err := sourcerepos.FindSourceRepositoryWithoutProvider(jxClient, ns, owner, repo)
	if err != nil {
		return errors.Wrap(err, "cannot determine git source URL")
	}
	if sr == nil {
		return fmt.Errorf("could not find existing SourceRepository for owner %s and repo %s", owner, repo)
	}

	sourceURL, err := sourcerepos.GetRepositoryGitURL(sr)
	if err != nil {
		return errors.Wrapf(err, "cannot generate the git URL from SourceRepository %s", sr.Name)
	}
	if sourceURL == "" {
		return fmt.Errorf("no git URL returned from SourceRepository %s", sr.Name)
	}

	log.Logger().Debug("creating meta pipeline client")
	client, err := metapipeline.NewMetaPipelineClient()
	if err != nil {
		return errors.Wrap(err, "unable to create meta pipeline client")
	}

	pullRef := metapipeline.NewPullRef(sourceURL, branch, "")
	pipelineKind := o.determinePipelineKind(branch)
	envVarMap, err := stringhelpers.ExtractKeyValuePairs(o.CustomEnvs, "=")
	if err != nil {
		return errors.Wrap(err, "unable to parse env variables")
	}

	labelMap, err := stringhelpers.ExtractKeyValuePairs(o.CustomLabels, "=")
	if err != nil {
		return errors.Wrap(err, "unable to parse label variables")
	}

	pipelineCreateParam := metapipeline.PipelineCreateParam{
		PullRef:        pullRef,
		PipelineKind:   pipelineKind,
		Context:        o.Context,
		EnvVariables:   envVarMap,
		Labels:         labelMap,
		ServiceAccount: o.ServiceAccount,
	}

	pipelineActivity, tektonCRDs, err := client.Create(pipelineCreateParam)
	if err != nil {
		return errors.Wrap(err, "unable to create Tekton CRDs")
	}

	err = client.Apply(pipelineActivity, tektonCRDs)
	if err != nil {
		return errors.Wrap(err, "unable to apply Tekton CRDs")
	}

	err = client.Close()
	if err != nil {
		log.Logger().Errorf("unable to close meta pipeline client: %s", err.Error())
	}

	return nil
}

func (o *Options) determinePipelineKind(branch string) metapipeline.PipelineKind {
	if o.PipelineKind != "" {
		return metapipeline.StringToPipelineKind(o.PipelineKind)
	}
	var kind metapipeline.PipelineKind

	// `jx pipeline start` will only always trigger a release or feature pipeline. Not sure whether there is a way
	// to configure your release branch atm. Using a constant here (HF)
	if branch == releaseBranchName {
		kind = metapipeline.ReleasePipeline
	} else {
		kind = metapipeline.FeaturePipeline
	}
	return kind
}

// pipelineNames returns the pipeline names to trigger
func (o *Options) pipelineNames(cfg *config.Config) []string {
	var answer []string
	for k := range cfg.Postsubmits {
		answer = append(answer, k)
	}

	// lets handle in repo configurations
	if cfg.InRepoConfig.Enabled != nil {
		for k := range cfg.InRepoConfig.Enabled {
			// lets ignore orgs or *
			if strings.Contains(k, "/") {
				answer = append(answer, k)
			}
		}
	}
	sort.Strings(answer)
	return answer
}
