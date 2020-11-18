package start

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jenkins-x/go-scm/scm"
	"github.com/jenkins-x/jx-api/v3/pkg/client/clientset/versioned"
	"github.com/jenkins-x/jx-helpers/v3/pkg/gitclient/giturl"
	"github.com/jenkins-x/jx-helpers/v3/pkg/input"
	"github.com/jenkins-x/jx-helpers/v3/pkg/input/inputfactory"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/jxclient"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/naming"
	"github.com/jenkins-x/jx-helpers/v3/pkg/options"
	"github.com/jenkins-x/jx-helpers/v3/pkg/scmhelpers"
	"github.com/jenkins-x/jx-helpers/v3/pkg/stringhelpers"
	"github.com/jenkins-x/jx-helpers/v3/pkg/termcolor"
	"github.com/jenkins-x/jx-pipeline/pkg/constants"
	"github.com/jenkins-x/jx-pipeline/pkg/lighthouses"
	"github.com/jenkins-x/jx-pipeline/pkg/sourcerepos"
	"github.com/jenkins-x/jx-pipeline/pkg/tektonlog"
	"github.com/jenkins-x/jx-pipeline/pkg/triggers"
	"github.com/jenkins-x/lighthouse/pkg/apis/lighthouse/v1alpha1"
	lhclient "github.com/jenkins-x/lighthouse/pkg/client/clientset/versioned"
	"github.com/jenkins-x/lighthouse/pkg/config"
	"github.com/jenkins-x/lighthouse/pkg/config/job"
	"github.com/jenkins-x/lighthouse/pkg/jobutil"
	"github.com/jenkins-x/lighthouse/pkg/launcher"
	"github.com/jenkins-x/lighthouse/pkg/plugins"
	"github.com/jenkins-x/lighthouse/pkg/triggerconfig/inrepo"
	"github.com/pkg/errors"
	"k8s.io/client-go/kubernetes"

	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/helper"

	"github.com/spf13/cobra"

	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/templates"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
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
	GitUsername         string
	GitToken            string
	Wait                bool
	Tail                bool
	WaitDuration        time.Duration
	PollPeriod          time.Duration
	KubeClient          kubernetes.Interface
	JXClient            versioned.Interface
	LHClient            lhclient.Interface
	Input               input.Interface

	// meta pipeline options
	Context          string
	CustomLabels     []string
	CustomEnvs       []string
	CustomParameters []string

	// ScmClients cache of Scm Clients mostly used for testing
	ScmClients         map[string]*scm.Client
	customParameterMap map[string]string
}

var (
	info = termcolor.ColorInfo

	cmdLong = templates.LongDesc(`
		Starts the pipeline build.

`)

	cmdExample = templates.Examples(`
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
		Long:    cmdLong,
		Example: cmdExample,
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
	cmd.Flags().StringVarP(&o.GitToken, "git-token", "", "", "the git token used to access the git repository for in-repo configurations in lighthouse")
	cmd.Flags().StringVarP(&o.GitUsername, "git-username", "", "", "the git username used to access the git repository for in-repo configurations in lighthouse")
	cmd.Flags().StringArrayVarP(&o.CustomLabels, "label", "l", nil, "List of custom labels to be applied to the generated PipelineRun (can be use multiple times)")
	cmd.Flags().StringArrayVarP(&o.CustomEnvs, "env", "e", nil, "List of custom environment variables to be applied to the generated PipelineRun that are created (can be use multiple times)")
	cmd.Flags().StringArrayVarP(&o.CustomParameters, "param", "", nil, "List of name=value PipelineRun parameters passed into the ligthhousejob which add or override any parameter values in the lighthouse postsubmit configuration")
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
	o.LHClient, err = lighthouses.LazyCreateLHClient(o.LHClient)
	if err != nil {
		return errors.Wrapf(err, "failed to create the lighthouse client")
	}

	if o.Input == nil {
		o.Input = inputfactory.NewInput(&o.BaseOptions)
	}
	o.customParameterMap = map[string]string{}
	for _, cp := range o.CustomParameters {
		paths := strings.SplitN(cp, "=", 2)
		if len(paths) != 2 {
			return options.InvalidOptionf("param", cp, "should be of the form 'name=value'")
		}
		o.customParameterMap[paths[0]] = paths[1]
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

	ctx := o.GetContext()
	names, cfg, err := o.getFilteredTriggerNames(ctx, o.KubeClient, o.Namespace)
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
		err = o.createLighthouseJob(a, cfg)
		if err != nil {
			return err
		}
	}
	return nil
}

func (o *Options) getFilteredTriggerNames(ctx context.Context, kubeClient kubernetes.Interface, ns string) ([]string, *config.Config, error) {
	end := time.Now().Add(o.WaitDuration)
	name := o.LighthouseConfigMap
	logWaiting := false

	for {
		cfg, err := triggers.LoadLighthouseConfig(ctx, kubeClient, ns, name, o.Wait)
		if err != nil {
			return nil, cfg, errors.Wrapf(err, "failed to load lighthouse config")
		}
		names := o.pipelineNames(cfg)
		names = stringhelpers.StringsContaining(names, o.Filter)

		if len(names) > 0 || !o.Wait {
			return names, cfg, nil
		}

		if time.Now().After(end) {
			return nil, cfg, errors.Errorf("failed to find trigger in the lighthouse configuration in ConfigMap %s in namespace %s matching filter: '%s' within %s", name, ns, o.Filter, o.WaitDuration.String())
		}

		if !logWaiting {
			logWaiting = true
			log.Logger().Infof("waiting up to %s for a trigger to be added to the lighthouse configuration in ConfigMap %s in namespace %s matching filter: '%s'", o.WaitDuration.String(), name, ns, o.Filter)
		}
		time.Sleep(o.PollPeriod)
	}
}

func (o *Options) createLighthouseJob(jobName string, cfg *config.Config) error {
	ctx := o.GetContext()

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
	ns := o.Namespace

	jobType := job.PostsubmitJob

	fullName := scm.Join(owner, repo)

	sr, err := sourcerepos.FindSourceRepositoryWithoutProvider(ctx, o.JXClient, ns, owner, repo)
	if err != nil {
		return errors.Wrapf(err, "failed to find the SourceRepository %s", fullName)
	}
	if sr == nil {
		return errors.Errorf("could not find a SourceRepository with owner %s name %s in namespace %s", owner, repo, ns)
	}

	gitServerURL := sr.Spec.Provider
	if gitServerURL == "" {
		var gitInfo *giturl.GitRepository
		gitInfo, err = giturl.ParseGitURL(sr.Spec.HTTPCloneURL)
		if err != nil {
			return errors.Wrapf(err, "failed to parse git clone URL %s", sr.Spec.HTTPCloneURL)
		}
		gitServerURL = gitInfo.HostURL()
	}

	gitKind := sr.Spec.ProviderKind
	if gitKind == "" {
		gitKind = giturl.SaasGitKind(gitServerURL)
	}

	f := scmhelpers.Factory{
		GitServerURL: gitServerURL,
		GitUsername:  o.GitUsername,
		GitToken:     o.GitToken,
		GitKind:      gitKind,
	}
	var scmClient *scm.Client
	if o.ScmClients != nil {
		scmClient = o.ScmClients[gitServerURL]
	}
	if scmClient == nil {
		scmClient, err = f.Create()
		if err != nil {
			return errors.Wrapf(err, "failed to create an ScmClient for %s", fullName)
		}
	}

	commit, _, err := scmClient.Git.FindCommit(ctx, fullName, branch)
	if err != nil {
		return errors.Wrapf(err, "failed to find commit on repo %s for branch %s", fullName, branch)
	}
	if commit == nil {
		return errors.Errorf("no commit on repo %s for branch %s", fullName, branch)
	}
	if cfg.InRepoConfigEnabled(fullName) {
		pluginCfg := &plugins.Configuration{
			Plugins: map[string][]string{
				fullName: {"trigger"},
			},
		}

		scmProvider := lighthouses.NewScmProvider(ctx, scmClient)
		cfg, _, err = inrepo.Generate(scmProvider, cfg, pluginCfg, owner, repo, "")
		if err != nil {
			return errors.Wrapf(err, "failed to calculate in repo configuration")
		}
	}

	postsubmits := cfg.Postsubmits[fullName]
	if len(postsubmits) == 0 {
		return errors.Errorf("could not find Postsubmit for repository %s", fullName)
	}

	// TODO pick the first one for now?
	postsubmit := postsubmits[0]

	pipelineRunParams := o.combineWithCustomParameters(postsubmit.PipelineRunParams)

	lhjob := &v1alpha1.LighthouseJob{
		Spec: v1alpha1.LighthouseJobSpec{
			Type:  jobType,
			Agent: job.TektonPipelineAgent,
			//Namespace: ns,
			Job: postsubmit.Name,
			Refs: &v1alpha1.Refs{
				Org:      owner,
				Repo:     repo,
				RepoLink: sr.Spec.URL,
				BaseRef:  branch,
				BaseSHA:  commit.Sha,
				BaseLink: commit.Link,
				CloneURI: sr.Spec.HTTPCloneURL,
			},
			ExtraRefs: nil,
			Context:   postsubmit.Context,
			//RerunCommand:      postsubmit.RerunCommand,
			MaxConcurrency:    postsubmit.MaxConcurrency,
			PipelineRunSpec:   postsubmit.PipelineRunSpec,
			PipelineRunParams: pipelineRunParams,
		},
	}

	lhjob.Labels, lhjob.Annotations = jobutil.LabelsAndAnnotationsForSpec(lhjob.Spec, nil, nil)
	lhjob.GenerateName = naming.ToValidName(owner+"-"+repo) + "-"

	launchClient := launcher.NewLauncher(o.LHClient, o.Namespace)
	lhjob, err = launchClient.Launch(lhjob)
	if err != nil {
		return errors.Wrapf(err, "failed to create lighthousejob %s in namespace %s", lhjob.Name, ns)
	}

	log.Logger().Infof("created lighthousejob %s in namespace %s", info(lhjob.Name), info(ns))
	return nil
}

func (o *Options) combineWithCustomParameters(params []job.PipelineRunParam) []job.PipelineRunParam {
	for name, value := range o.customParameterMap {
		found := false
		for i := range params {
			p := &params[i]
			if p.Name == name {
				p.ValueTemplate = value
				found = true
				break
			}
		}
		if !found {
			params = append(params, job.PipelineRunParam{
				Name:          name,
				ValueTemplate: value,
			})
		}
	}
	return params
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
