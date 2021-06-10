package env

import (
	"fmt"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/tektonlog"
	v1 "github.com/jenkins-x/jx-api/v4/pkg/apis/jenkins.io/v1"
	"github.com/jenkins-x/jx-api/v4/pkg/client/clientset/versioned"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/v3/pkg/input"
	"github.com/jenkins-x/jx-helpers/v3/pkg/input/inputfactory"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/jxclient"
	"github.com/jenkins-x/jx-helpers/v3/pkg/options"
	"github.com/jenkins-x/jx-helpers/v3/pkg/termcolor"
	"github.com/jenkins-x/jx-kube-client/v3/pkg/kubeclient"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
	"github.com/pkg/errors"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	tektonclient "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sort"
	"strings"

	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/templates"
	"github.com/spf13/cobra"
)

// Options the command line options
type Options struct {
	options.BaseOptions

	Args         []string
	Format       string
	Namespace    string
	BuildFilter  tektonlog.BuildPodInfoFilter
	KubeClient   kubernetes.Interface
	JXClient     versioned.Interface
	TektonClient tektonclient.Interface
	TektonLogger *tektonlog.TektonLogger
	Input        input.Interface
}

type PipelinePod struct {
	PodName      string
	ActivitySpec *v1.PipelineActivitySpec
	PipelineRuns []*v1beta1.PipelineRun
}

var (
	cmdLong = templates.LongDesc(`
		Display the Pipeline step environment variables for a step in a chosen pipeline pod

`)

	cmdExample = templates.Examples(`
		# Pick the pipeline pod and step to view the environment variables
		jx pipeline env

		# Pick the pipeline pod for a repository and step to view the environment variables
		jx pipeline env --repo cheese

		# Pick the pipeline pod for a repository, a given Pull Request and a step to view the environment variables
		jx pipeline env --repo cheese --branch PR-1234

		# Generate IDEA based environment variable output you can copy/paste into the Run/Debug UI
		jx pipeline env -t idea

	`)
)

// NewCmdPipelineEnv creates the command
func NewCmdPipelineEnv() (*cobra.Command, *Options) {
	o := &Options{}

	cmd := &cobra.Command{
		Use:     "env [flags]",
		Short:   "Displays the environment variables for a step in a chosen pipeline pod",
		Long:    cmdLong,
		Example: cmdExample,
		Aliases: []string{"environment"},
		Run: func(cmd *cobra.Command, args []string) {
			o.Args = args
			err := o.Run()
			helper.CheckErr(err)
		},
	}
	cmd.Flags().StringVarP(&o.Namespace, "namespace", "n", "", "The namespace to look for the build pods. Defaults to the current namespace")
	cmd.Flags().StringVarP(&o.Format, "format", "t", "shell", "The output format. Valid values are 'shell' or 'idea'")

	o.BaseOptions.AddBaseFlags(cmd)
	o.BuildFilter.AddFlags(cmd)
	return cmd, o
}

// Validate verifies things are setup correctly
func (o *Options) Validate() error {
	err := o.BuildFilter.Validate()
	if err != nil {
		return err
	}

	o.KubeClient, o.Namespace, err = kube.LazyCreateKubeClientAndNamespace(o.KubeClient, o.Namespace)
	if err != nil {
		return errors.Wrapf(err, "failed to create kube client")
	}
	o.JXClient, err = jxclient.LazyCreateJXClient(o.JXClient)
	if err != nil {
		return errors.Wrapf(err, "failed to create the jx client")
	}

	if o.TektonClient != nil {
		return nil
	}

	f := kubeclient.NewFactory()
	cfg, err := f.CreateKubeConfig()
	if err != nil {
		return errors.Wrap(err, "failed to get kubernetes config")
	}
	o.TektonClient, err = tektonclient.NewForConfig(cfg)
	if err != nil {
		return errors.Wrap(err, "error building tekton client")
	}

	if o.TektonLogger == nil {
		o.TektonLogger = &tektonlog.TektonLogger{
			KubeClient:   o.KubeClient,
			TektonClient: o.TektonClient,
			JXClient:     o.JXClient,
			Namespace:    o.Namespace,
		}
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

	ctx := o.GetContext()

	names, paMap, prMap, err := o.TektonLogger.GetTektonPipelinesWithActivePipelineActivity(ctx, &o.BuildFilter)
	if err != nil {
		return err
	}

	var filter string
	if len(o.Args) > 0 {
		filter = o.Args[0]
	} else {
		filter = o.BuildFilter.Filter
	}

	var filteredNames []string
	for _, n := range names {
		if strings.Contains(strings.ToLower(n), strings.ToLower(filter)) {
			filteredNames = append(filteredNames, n)
		}
	}

	var pipelineNames []string
	m := map[string]*PipelinePod{}
	for _, name := range filteredNames {
		pa := paMap[name]
		if pa == nil {
			continue
		}
		pr := prMap[name]

		build := &pa.Spec
		podName := pa.Labels["podName"]

		name := fmt.Sprintf("%s/%s/%s #%s : %s : %s", build.GitOwner, build.GitRepository, build.GitBranch, build.Build, build.Context, string(build.Status))
		pipelineNames = append(pipelineNames, name)
		m[name] = &PipelinePod{
			PodName:      podName,
			ActivitySpec: &pa.Spec,
			PipelineRuns: pr,
		}
	}

	sort.Strings(pipelineNames)

	name, err := o.Input.PickNameWithDefault(pipelineNames, "Pick the pipeline you wish to view the environment for: ", "", "Please select the pipeline pod you wish to view")
	if err != nil {
		return errors.Wrapf(err, "failed to pick a pipeline")
	}
	pp := m[name]
	if pp == nil {
		return errors.Errorf("could not find Pipeline Pod for name: %s", name)
	}

	return o.viewEnvironment(name, pp)
}

func (o *Options) viewEnvironment(name string, pp *PipelinePod) error {
	log.Logger().Debugf("picked pipeline pod %s", pp.PodName)

	if len(pp.PipelineRuns) == 0 {
		return errors.Errorf("no PipelineRun objects for pipeline: %s pod: %s", name, pp.PodName)
	}
	pr := pp.PipelineRuns[0]
	if pr == nil {
		return errors.Errorf("no PipelineRun objects for pipeline: %s pod: %s", name, pp.PodName)
	}

	ps := pr.Spec.PipelineSpec
	if ps == nil {
		return errors.Errorf("Pipeline: %s has no PipelineSpec for PipelineRun: %s pod: %s", name, pr.Name)
	}

	var taskNames []string
	tasks := map[string]*v1beta1.TaskSpec{}
	for i := range ps.Tasks {
		pt := &ps.Tasks[i]
		if pt.TaskSpec == nil {
			continue
		}
		ts := &pt.TaskSpec.TaskSpec
		taskNames = append(taskNames, pt.Name)
		tasks[pt.Name] = ts
	}
	taskName, err := o.Input.PickNameWithDefault(taskNames, "Pick the task you wish to view the environment for: ", "", "Please select the pipeline task you wish to view")
	if err != nil {
		return errors.Wrapf(err, "failed to pick a pipeline")
	}

	ts := tasks[taskName]
	if ts == nil {
		return errors.Errorf("could not find a TaskRun for PipelineRun %s and Task %s", pr.Name, taskName)
	}

	var stepNames []string
	for i := range ts.Steps {
		s := &ts.Steps[i]
		stepNames = append(stepNames, s.Name)
	}
	stepName, err := o.Input.PickNameWithDefault(stepNames, "Pick the step you wish to view the environment for: ", "", "Please select the step name you wish to view")
	if err != nil {
		return errors.Wrapf(err, "failed to pick a pipeline")
	}
	if stepName == "" {
		return errors.Errorf("did not choose step in TaskRun %s for PipelineRun %s and Task %s", taskName, pr.Name, taskName)
	}

	return o.viewVariables(name, pp, pr, ts, "step-"+stepName)
}

func (o *Options) viewVariables(name string, pp *PipelinePod, pr *v1beta1.PipelineRun, ts *v1beta1.TaskSpec, stepName string) error {
	ctx := o.GetContext()
	pod, err := o.KubeClient.CoreV1().Pods(o.Namespace).Get(ctx, pp.PodName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to load pod %s in namespace %s", pp.PodName, o.Namespace)
	}

	for i := range pod.Spec.Containers {
		c := &pod.Spec.Containers[i]
		if c.Name == stepName {
			envVars := map[string]string{}
			o.addEnvVarValues(envVars, c.Env, c.EnvFrom)
			return o.renderEnv(envVars)
		}
	}
	return errors.Errorf("could not find container name %s in pod %s", stepName, pp.PodName)
}

func (o *Options) addEnvVarValues(m map[string]string, env []corev1.EnvVar, from []corev1.EnvFromSource) {
	for _, e := range env {
		if e.ValueFrom != nil {
			continue
		}
		m[e.Name] = e.Value
	}

	for _, f := range from {
		// TODO
		if f.SecretRef != nil {
			log.Logger().Debugf("TODO load secret %s", f.SecretRef.Name)
		}
	}
}

func (o *Options) renderEnv(envVars map[string]string) error {
	if len(envVars) == 0 {
		log.Logger().Infof("no environment variables")
		return nil
	}
	var keys []string
	for k := range envVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	buf := &strings.Builder{}
	for _, k := range keys {
		v := envVars[k]
		o.logEnvVar(buf, k, v)
	}
	log.Logger().Infof(termcolor.ColorStatus(buf.String()))
	return nil
}

func (o *Options) logEnvVar(buf *strings.Builder, k string, v string) {
	switch o.Format {
	case "idea":
		if buf.Len() > 0 {
			buf.WriteString(";")
		}
		buf.WriteString(fmt.Sprintf("%s=%s", k, v))
	default:
		buf.WriteString(fmt.Sprintf("export %s=\"%s\"\n", k, v))
	}
}
