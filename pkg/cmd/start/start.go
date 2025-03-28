package start

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jenkins-x/jx-helpers/v3/pkg/cmdrunner"
	"github.com/jenkins-x/jx-helpers/v3/pkg/gitclient"
	"github.com/jenkins-x/jx-helpers/v3/pkg/gitclient/cli"
	"github.com/jenkins-x/jx-helpers/v3/pkg/gitclient/gitdiscovery"
	gitv2 "github.com/jenkins-x/lighthouse-client/pkg/git/v2"
	"github.com/sirupsen/logrus"
	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"

	"github.com/jenkins-x/lighthouse-client/pkg/filebrowser"

	"github.com/jenkins-x-plugins/jx-pipeline/pkg/constants"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/lighthouses"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/sourcerepos"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/tektonlog"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/triggers"
	"github.com/jenkins-x/go-scm/scm"
	"github.com/jenkins-x/jx-api/v4/pkg/client/clientset/versioned"
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
	"github.com/jenkins-x/lighthouse-client/pkg/apis/lighthouse/v1alpha1"
	lhclient "github.com/jenkins-x/lighthouse-client/pkg/client/clientset/versioned"
	"github.com/jenkins-x/lighthouse-client/pkg/config"
	"github.com/jenkins-x/lighthouse-client/pkg/config/job"
	"github.com/jenkins-x/lighthouse-client/pkg/jobutil"
	"github.com/jenkins-x/lighthouse-client/pkg/launcher"
	"github.com/jenkins-x/lighthouse-client/pkg/plugins"
	"github.com/jenkins-x/lighthouse-client/pkg/triggerconfig/inrepo"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/helper"

	"github.com/spf13/cobra"

	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/templates"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
)

// Options contains the command line options
type Options struct {
	options.BaseOptions
	lighthouses.ResolverOptions

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
	CatalogSHA          string
	File                string
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
	CustomEnvs       map[string]string
	CustomParameters []string

	// ScmClients cache of Scm Clients mostly used for testing
	ScmClients         map[string]*scm.Client
	customParameterMap map[string]string

	// file based starter
	Resolver      *inrepo.UsesResolver
	GitClient     gitclient.Interface
	CommandRunner cmdrunner.CommandRunner
}

var (
	logger = logrus.WithField("jx-pipeline", "start")
	info   = termcolor.ColorInfo

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

		# Start the given local pipeline file
		jx pipeline start -F .lighthouse/jenkins-x/mypipeline.yaml
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
		Run: func(_ *cobra.Command, args []string) {
			o.Args = args
			err := o.Run()
			helper.CheckErr(err)
		},
	}
	cmd.Flags().BoolVarP(&o.Tail, "tail", "t", false, "Tails the build log to the current terminal")
	cmd.Flags().StringVarP(&o.File, "file", "F", "", "The pipeline file to start")
	cmd.Flags().StringVarP(&o.Filter, "filter", "f", "", "Filters all the available jobs by those that contain the given text")
	cmd.Flags().StringVarP(&o.Context, "context", "c", "", "An optional context name to find the specific kind of postsubmit/presubmit if there are more than one triggers")
	cmd.Flags().StringVarP(&o.Branch, "branch", "", "", "The branch to start. If not specified then the default branch of the repository is used")
	cmd.Flags().StringVarP(&o.PipelineKind, "kind", "", "", "The kind of pipeline such as presubmit or post submit. If not specified defaults to postsubmit (i.e. release)")
	cmd.Flags().StringVar(&o.ServiceAccount, "service-account", tektonlog.DefaultPipelineSA, "The Kubernetes ServiceAccount to use to run the meta pipeline")
	cmd.Flags().StringVarP(&o.LighthouseConfigMap, "configmap", "", constants.LighthouseConfigMapName, "The name of the Lighthouse ConfigMap to find the trigger configurations")
	cmd.Flags().StringVarP(&o.GitToken, "git-token", "", "", "the git token used to access the git repository for in-repo configurations in lighthouse")
	cmd.Flags().StringVarP(&o.GitUsername, "git-username", "", "", "the git username used to access the git repository for in-repo configurations in lighthouse")
	cmd.Flags().StringArrayVarP(&o.CustomLabels, "label", "l", nil, "List of custom labels to be applied to the generated PipelineRun (can be use multiple times)")
	cmd.Flags().StringToStringVarP(&o.CustomEnvs, "env", "e", nil, "List of custom environment variables to be applied to the generated PipelineRun that are created (can be use multiple times)")
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
		return fmt.Errorf("failed to create kube client: %w", err)
	}
	o.JXClient, err = jxclient.LazyCreateJXClient(o.JXClient)
	if err != nil {
		return fmt.Errorf("failed to create the jx client: %w", err)
	}
	o.LHClient, err = lighthouses.LazyCreateLHClient(o.LHClient)
	if err != nil {
		return fmt.Errorf("failed to create the lighthouse client: %w", err)
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

	lighthouses.DefaultPipelineCatalogSHA(o.CatalogSHA)
	return nil
}

// Run implements this command
func (o *Options) Run() error {
	err := o.Validate()
	if err != nil {
		return fmt.Errorf("failed to validate options: %w", err)
	}

	if o.File != "" {
		return o.processFile(o.File)
	}

	args := o.Args
	ctx := o.GetContext()
	names, cfg, err := o.getFilteredTriggerNames(ctx, o.KubeClient, o.Namespace)
	if err != nil {
		return fmt.Errorf("failed to get trigger names: %w", err)
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
			return nil, cfg, fmt.Errorf("failed to load lighthouse config: %w", err)
		}
		names := o.pipelineNames(cfg)
		names = stringhelpers.StringsContaining(names, o.Filter)

		if len(names) > 0 || !o.Wait {
			return names, cfg, nil
		}

		if time.Now().After(end) {
			return nil, cfg, fmt.Errorf("failed to find trigger in the lighthouse configuration in ConfigMap %s in namespace %s matching filter: '%s' within %s", name, ns, o.Filter, o.WaitDuration.String())
		}

		if !logWaiting {
			logWaiting = true
			log.Logger().Infof("waiting up to %s for a trigger to be added to the lighthouse configuration in ConfigMap %s in namespace %s matching filter: '%s'", o.WaitDuration.String(), name, ns, o.Filter)
		}
		time.Sleep(o.PollPeriod)
	}
}

func (o *Options) processFile(path string) error {
	var err error
	if o.Resolver == nil {
		o.Resolver, err = o.ResolverOptions.CreateResolver()
		if err != nil {
			return fmt.Errorf("failed to create a UsesResolver: %w", err)
		}
	}

	pr, err := lighthouses.LoadEffectivePipelineRun(o.Resolver, path)
	if err != nil {
		return fmt.Errorf("failed to load %s: %w", path, err)
	}
	ns := o.Namespace
	if o.Context == "" {
		o.Context = "trigger"
	}
	if len(o.CustomEnvs) > 0 {
		o.addCustomEnvsToStepTemplate(pr.Spec.PipelineSpec)
	}
	jobName := o.Context
	jobType := job.PostsubmitJob

	dir := o.Dir
	if dir == "" {
		dir = "."
	}
	if o.CommandRunner == nil {
		o.CommandRunner = cmdrunner.QuietCommandRunner
	}
	if o.GitClient == nil {
		o.GitClient = cli.NewCLIClient("", o.CommandRunner)
	}
	if o.Branch == "" {
		o.Branch, err = gitclient.Branch(o.GitClient, dir)
		if err != nil {
			return fmt.Errorf("failed to detect the git branch: %w", err)
		}
	}
	sha, err := gitclient.GetLatestCommitSha(o.GitClient, dir)
	if err != nil {
		return fmt.Errorf("failed to get the current git commit sha: %w", err)
	}

	gitInfo, err := gitdiscovery.FindGitInfoFromDir(dir)
	if err != nil {
		return fmt.Errorf("failed to discover git url: %w", err)
	}
	gitURL := gitInfo.URL
	gitCloneURL := gitInfo.HttpsURL()
	owner := gitInfo.Organisation
	repo := gitInfo.Name

	// TODO no way to load these from a trigger if using the specific file...
	pipelineRunParams := o.combineWithCustomParameters(nil)

	lhjob := &v1alpha1.LighthouseJob{
		Spec: v1alpha1.LighthouseJobSpec{
			Type:  jobType,
			Agent: job.TektonPipelineAgent,
			// Namespace: ns,
			Job: jobName,
			Refs: &v1alpha1.Refs{
				Org:      owner,
				Repo:     repo,
				RepoLink: gitURL,
				BaseRef:  o.Branch,
				BaseSHA:  sha,
				// BaseLink: commit.Link,
				CloneURI: gitCloneURL,
			},
			ExtraRefs: nil,
			Context:   o.Context,
			// RerunCommand:      base.RerunCommand,
			// MaxConcurrency:    base.MaxConcurrency,
			PipelineRunSpec:   &pr.Spec,
			PipelineRunParams: pipelineRunParams,
		},
	}

	lhjob.Labels, lhjob.Annotations = jobutil.LabelsAndAnnotationsForSpec(lhjob.Spec, nil, nil)
	lhjob.GenerateName = naming.ToValidName(owner+"-"+repo) + "-"

	launchClient := launcher.NewLauncher(o.LHClient, o.Namespace)
	lhjob, err = launchClient.Launch(lhjob)
	if err != nil {
		return fmt.Errorf("failed to create lighthousejob for context %s of repo %s/%s in namespace %s: %w",
			jobName, owner, repo, ns, err)
	}

	log.Logger().Infof("created lighthousejob %s in namespace %s", info(lhjob.Name), info(ns))
	return nil
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
		branch = "HEAD"
	}
	ns := o.Namespace

	jobType := job.PostsubmitJob

	fullName := scm.Join(owner, repo)

	sr, err := sourcerepos.FindSourceRepositoryWithoutProvider(ctx,
		o.JXClient, ns, owner, repo)
	if err != nil {
		return fmt.Errorf("failed to find the SourceRepository %s: %w", fullName, err)
	}
	if sr == nil {
		return fmt.Errorf("could not find a SourceRepository with owner %s name %s in namespace %s", owner, repo, ns)
	}

	gitServerURL := sr.Spec.Provider
	var gitInfo *giturl.GitRepository
	if gitServerURL == "" {

		gitInfo, err = giturl.ParseGitURL(sr.Spec.HTTPCloneURL)
		if err != nil {
			return fmt.Errorf("failed to parse git clone URL %s: %w", sr.Spec.HTTPCloneURL, err)
		}
		gitServerURL = gitInfo.HostURL()
	} else {
		// if failed to parse we ignore and try use default values with github...
		gitInfo, _ = giturl.ParseGitURL(gitServerURL)
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
			return fmt.Errorf("failed to create an ScmClient for %s: %w", fullName, err)
		}
	}

	commit, _, err := scmClient.Git.FindCommit(ctx, fullName, branch)
	if err != nil {
		return fmt.Errorf("failed to find commit on repo %s for branch %s: %w", fullName, branch, err)
	}
	if commit == nil {
		return fmt.Errorf("no commit on repo %s for branch %s", fullName, branch)
	}
	//nolint:govet
	if cfg.InRepoConfigEnabled(fullName) {
		pluginCfg := &plugins.Configuration{
			Plugins: map[string][]string{
				fullName: {"trigger"},
			},
		}

		configureOpts := func(opts *gitv2.ClientFactoryOpts) {
			opts.Token = func() []byte {
				return []byte(f.GitToken)
			}
			opts.GitUser = func() (name, email string, err error) {
				name = f.GitUsername
				return
			}
			opts.Username = func() (login string, err error) {
				login = f.GitUsername
				return
			}
			opts.Host = gitInfo.Host
			opts.Scheme = gitInfo.Scheme
		}
		gitFactory, err := gitv2.NewClientFactory(configureOpts)
		if err != nil {
			return fmt.Errorf("failed to create git factory: %w", err)
		}
		fb := filebrowser.NewFileBrowserFromGitClient(gitFactory)

		fileBrowsers, err := filebrowser.NewFileBrowsers(gitServerURL, fb)
		if err != nil {
			return fmt.Errorf("failed to create file browsers: %w", err)
		}
		cache := inrepo.NewResolverCache()
		cfg, _, err = inrepo.Generate(fileBrowsers, filebrowser.NewFetchCache(), cache, cfg, pluginCfg, owner, repo, "")
		if err != nil {
			return fmt.Errorf("failed to calculate in repo configuration: %w", err)
		}
	}

	contextName, base, err := o.pickTrigger(cfg, fullName)
	if err != nil {
		return fmt.Errorf("failed to pick trigger to start: %w", err)
	}
	pipelineRunParams := o.combineWithCustomParameters(base.PipelineRunParams)
	err = base.LoadPipeline(logger)
	if err != nil {
		return fmt.Errorf("failed to load base pipeline: %w", err)
	}
	lhjob := &v1alpha1.LighthouseJob{
		Spec: v1alpha1.LighthouseJobSpec{
			Type:  jobType,
			Agent: job.TektonPipelineAgent,
			// Namespace: ns,
			Job: base.Name,
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
			Context:   contextName,
			// RerunCommand:      base.RerunCommand,
			MaxConcurrency:    base.MaxConcurrency,
			PipelineRunSpec:   base.PipelineRunSpec,
			PipelineRunParams: pipelineRunParams,
		},
	}

	// These jobs were not created by event trigger from git, but started using jx pipeline start
	extraLabels := map[string]string{
		"external-trigger": "true",
	}

	lhjob.Labels, lhjob.Annotations = jobutil.LabelsAndAnnotationsForSpec(lhjob.Spec, extraLabels, nil)
	lhjob.GenerateName = naming.ToValidName(owner+"-"+repo) + "-"

	launchClient := launcher.NewLauncher(o.LHClient, o.Namespace)
	lhjob, err = launchClient.Launch(lhjob)
	if err != nil {
		return fmt.Errorf("failed to create lighthousejob in namespace %s: %w", ns, err)
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

func (o *Options) addCustomEnvsToStepTemplate(spec *pipelinev1.PipelineSpec) {
	for name, value := range o.CustomEnvs {
		for i := range spec.Tasks {
			stepTemplate := spec.Tasks[i].TaskSpec.StepTemplate
			stepTemplate.Env = append(stepTemplate.Env, v1.EnvVar{
				Name:  name,
				Value: value,
			})
		}
	}
}

// pipelineNames returns the pipeline names to trigger
func (o *Options) pipelineNames(cfg *config.Config) []string {
	var answer []string
	// TODO: Support the other job types as well. Probably through a --job-type option
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

func (o *Options) pickTrigger(cfg *config.Config, fullName string) (string, job.Base, error) {
	var names []string
	kind := strings.ToLower(o.PipelineKind)
	if strings.HasPrefix(kind, "pull") || strings.HasPrefix(kind, "pre") {
		triggers := cfg.Presubmits[fullName]
		if len(triggers) == 0 {
			return "", job.Base{}, fmt.Errorf("could not find presubmit for repository %s", fullName)
		}
		if o.Context == "" {
			trigger := triggers[0]
			return trigger.Context, trigger.Base, nil
		}
		for k := range triggers {
			// naming this trigger fails sonarcloud checks!
			t := triggers[k]
			name := t.Context
			if name == o.Context {
				return name, t.Base, nil
			}
			names = append(names, name)
		}
		return "", job.Base{}, fmt.Errorf("no presubmit for context %s found. Have contexts %s", o.Context, strings.Join(names, " "))
	}
	triggers := cfg.Postsubmits[fullName]
	if len(triggers) == 0 {
		return "", job.Base{}, fmt.Errorf("could not find postsubmit for repository %s", fullName)
	}
	if o.Context == "" {
		trigger := triggers[0]
		return trigger.Context, trigger.Base, nil
	}
	for k := range triggers {
		trigger := triggers[k]
		name := trigger.Context
		if name == o.Context {
			return name, trigger.Base, nil
		}
		names = append(names, name)
	}
	return "", job.Base{}, fmt.Errorf("no postsubmit for context %s found. Have contexts %s", o.Context, strings.Join(names, " "))
}
