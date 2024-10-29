package env

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jenkins-x-plugins/jx-pipeline/pkg/tektonlog"
	v1 "github.com/jenkins-x/jx-api/v4/pkg/apis/jenkins.io/v1"
	"github.com/jenkins-x/jx-api/v4/pkg/client/clientset/versioned"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/v3/pkg/input"
	"github.com/jenkins-x/jx-helpers/v3/pkg/input/inputfactory"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/jxclient"
	"github.com/jenkins-x/jx-helpers/v3/pkg/options"
	"github.com/jenkins-x/jx-helpers/v3/pkg/stringhelpers"
	"github.com/jenkins-x/jx-helpers/v3/pkg/termcolor"
	"github.com/jenkins-x/jx-kube-client/v3/pkg/kubeclient"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"

	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	tektonclient "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"

	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/templates"
	"github.com/spf13/cobra"
)

// Options the command line options
type Options struct {
	options.BaseOptions

	Args         []string
	Exclude      []string
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
		Run: func(_ *cobra.Command, args []string) {
			o.Args = args
			err := o.Run()
			helper.CheckErr(err)
		},
	}
	cmd.Flags().StringVarP(&o.Namespace, "namespace", "n", "", "The namespace to look for the build pods. Defaults to the current namespace")
	cmd.Flags().StringVarP(&o.Format, "format", "t", "shell", "The output format. Valid values are 'shell' or 'idea'")
	cmd.Flags().StringArrayVarP(&o.Exclude, "exclude", "x", nil, "The environment variable names to exclude.")

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
		return fmt.Errorf("failed to create kube client: %w", err)
	}
	o.JXClient, err = jxclient.LazyCreateJXClient(o.JXClient)
	if err != nil {
		return fmt.Errorf("failed to create the jx client: %w", err)
	}

	if o.TektonClient != nil {
		return nil
	}

	f := kubeclient.NewFactory()
	cfg, err := f.CreateKubeConfig()
	if err != nil {
		return fmt.Errorf("failed to get kubernetes config: %w", err)
	}
	o.TektonClient, err = tektonclient.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("error building tekton client: %w", err)
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
		return fmt.Errorf("failed to validate options: %w", err)
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
		return fmt.Errorf("failed to pick a pipeline: %w", err)
	}
	pp := m[name]
	if pp == nil {
		return fmt.Errorf("could not find Pipeline Pod for name: %s", name)
	}

	return o.viewEnvironment(name, pp)
}

func (o *Options) viewEnvironment(name string, pp *PipelinePod) error {
	log.Logger().Debugf("picked pipeline pod %s", pp.PodName)

	if len(pp.PipelineRuns) == 0 {
		return fmt.Errorf("no PipelineRun objects for pipeline: %s pod: %s", name, pp.PodName)
	}
	pr := pp.PipelineRuns[0]
	if pr == nil {
		return fmt.Errorf("no PipelineRun objects for pipeline: %s pod: %s", name, pp.PodName)
	}

	ps := pr.Spec.PipelineSpec
	if ps == nil {
		return fmt.Errorf("Pipeline: %s has no PipelineSpec for PipelineRun: %s pod: %s", name, pr.Name, pp.PodName)
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
		return fmt.Errorf("failed to pick a pipeline: %w", err)
	}

	ts := tasks[taskName]
	if ts == nil {
		return fmt.Errorf("could not find a TaskRun for PipelineRun %s and Task %s", pr.Name, taskName)
	}

	var stepNames []string
	for i := range ts.Steps {
		s := &ts.Steps[i]
		stepNames = append(stepNames, s.Name)
	}
	stepName, err := o.Input.PickNameWithDefault(stepNames, "Pick the step you wish to view the environment for: ", "", "Please select the step name you wish to view")
	if err != nil {
		return fmt.Errorf("failed to pick a pipeline: %w", err)
	}
	if stepName == "" {
		return fmt.Errorf("did not choose step in TaskRun %s for PipelineRun %s and Task %s", taskName, pr.Name, taskName)
	}

	return o.viewVariables(pp, "step-"+stepName)
}

func (o *Options) viewVariables(pp *PipelinePod, stepName string) error {
	ctx := o.GetContext()
	pod, err := o.KubeClient.CoreV1().Pods(o.Namespace).Get(ctx, pp.PodName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to load pod %s in namespace %s: %w", pp.PodName, o.Namespace, err)
	}
	if pod.Name == "" {
		pod.Name = pp.PodName
	}
	envVars, err := o.PodEnvVars(pod, stepName)
	if err != nil {
		return fmt.Errorf("failed to get environment variables: %w", err)
	}
	return o.renderEnv(envVars)
}

// PodEnvVars returns the pod environment variables for the given container name
func (o *Options) PodEnvVars(pod *corev1.Pod, containerName string) (map[string]string, error) {
	for i := range pod.Spec.Containers {
		c := &pod.Spec.Containers[i]
		if c.Name == containerName {
			envVars := map[string]string{}
			err := o.addEnvVarValues(envVars, c.Env, c.EnvFrom)
			if err != nil {
				return nil, fmt.Errorf("failed to add env vars: %w", err)
			}
			return envVars, nil
		}
	}
	return nil, fmt.Errorf("could not find container name %s in pod %s", containerName, pod.Name)
}

func (o *Options) addEnvVarValues(m map[string]string, env []corev1.EnvVar, from []corev1.EnvFromSource) error {
	for _, e := range env {
		from := e.ValueFrom
		envVar := e.Name
		if e.Value != "" {
			m[envVar] = e.Value
			continue
		}
		if from != nil {
			if from.ConfigMapKeyRef != nil {
				optional := asBool(from.ConfigMapKeyRef.Optional)
				refName := from.ConfigMapKeyRef.LocalObjectReference.Name
				data, err := o.getConfigData(refName, optional)
				if err != nil {
					return err
				}
				err = addEnvValueFrom(m, envVar, from.ConfigMapKeyRef.Key, data)
				if err != nil {
					return fmt.Errorf("failed to add varables from ConfigMap %s: %w", refName, err)
				}
				continue
			}
			if from.SecretKeyRef != nil {
				optional := asBool(from.SecretKeyRef.Optional)
				refName := from.SecretKeyRef.LocalObjectReference.Name
				data, err := o.getSecretData(refName, optional)
				if err != nil {
					return err
				}
				err = addEnvValueFrom(m, envVar, from.SecretKeyRef.Key, data)
				if err != nil {
					return fmt.Errorf("failed to add varables from Secret %s: %w", refName, err)
				}
				continue
			}
		}
	}
	for _, f := range from {
		if f.SecretRef != nil {
			name := f.SecretRef.Name
			if name == "" {
				return fmt.Errorf("missing secret ref name")
			}
			optional := asBool(f.SecretRef.Optional)
			data, err := o.getSecretData(name, optional)
			if err != nil {
				return err
			}
			err = addEnvFromSource(m, f.Prefix, data)
			if err != nil {
				return fmt.Errorf("failed to add varables from Secret %s: %w", name, err)
			}

		} else if f.ConfigMapRef != nil {
			name := f.SecretRef.Name
			if name == "" {
				return fmt.Errorf("missing config ref name")
			}
			optional := asBool(f.SecretRef.Optional)
			data, err := o.getConfigData(name, optional)
			if err != nil {
				return err
			}
			err = addEnvFromSource(m, f.Prefix, data)
			if err != nil {
				return fmt.Errorf("failed to add varables from ConfigMap %s: %w", name, err)
			}
		}
	}
	return nil
}

func asBool(optional *bool) bool {
	if optional != nil {
		return *optional
	}
	return false
}

func (o *Options) getSecretData(name string, optional bool) (map[string]string, error) {
	ctx := o.GetContext()
	ns := o.Namespace
	r, err := o.KubeClient.CoreV1().Secrets(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			if optional {
				log.Logger().Debugf("no Secret called %s in namespace %s so ignoring", name, ns)
				return nil, nil
			}
			return nil, fmt.Errorf("no Secret called %s in namespace %s so ignoring", name, ns)
		}
		return nil, fmt.Errorf("failed to find Secret %s in namespace %s so ignoring: %w", name, ns, err)
	}
	return secretToMap(r), nil
}

func (o *Options) getConfigData(name string, optional bool) (map[string]string, error) {
	ctx := o.GetContext()
	ns := o.Namespace
	r, err := o.KubeClient.CoreV1().ConfigMaps(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			if optional {
				log.Logger().Debugf("no ConfigMap called %s in namespace %s so ignoring", name, ns)
				return nil, nil
			}
			return nil, fmt.Errorf("no ConfigMap called %s in namespace %s so ignoring", name, ns)
		}
		return nil, fmt.Errorf("failed to find ConfigMap %s in namespace %s so ignoring: %w", name, ns, err)
	}
	if r == nil {
		return nil, nil
	}
	return r.Data, nil
}

func addEnvValueFrom(m map[string]string, name, key string, data map[string]string) error {
	if data == nil {
		return nil
	}
	if name == "" {
		return addEnvFromSource(m, "", data)
	}
	if key == "" {
		return fmt.Errorf("missing key for valueFrom for name %s", name)
	}
	m[name] = data[key]
	return nil
}

func addEnvFromSource(m map[string]string, prefix string, envFromValues map[string]string) error {
	if envFromValues == nil {
		return nil
	}
	for k, v := range envFromValues {
		if prefix != "" {
			k = prefix + k
		}
		if len(validation.IsEnvVarName(k)) == 0 {
			m[k] = v
		}
	}
	return nil
}

func secretToMap(r *corev1.Secret) map[string]string {
	if r == nil {
		return nil
	}
	m := map[string]string{}
	if r.Data != nil {
		for k, v := range r.Data {
			m[k] = string(v)
		}
	}
	if r.StringData != nil {
		for k, v := range r.StringData {
			m[k] = v
		}
	}
	return m
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
		if stringhelpers.StringArrayIndex(o.Exclude, k) >= 0 {
			continue
		}
		o.logEnvVar(buf, k, v)
	}
	log.Logger().Info(termcolor.ColorStatus(buf.String()))
	return nil
}

func (o *Options) logEnvVar(buf *strings.Builder, k, v string) {
	switch o.Format {
	case "idea":
		if buf.Len() > 0 {
			buf.WriteString(";")
		}
		_, _ = fmt.Fprintf(buf, "%s=%s", k, v)
	default:
		_, _ = fmt.Fprintf(buf, "export %q=%q\n", k, v)
	}
}
