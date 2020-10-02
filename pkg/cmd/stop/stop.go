package stop

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jenkins-x/jx-api/pkg/client/clientset/versioned"
	"github.com/jenkins-x/jx-helpers/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/pkg/input"
	"github.com/jenkins-x/jx-helpers/pkg/input/inputfactory"
	"github.com/jenkins-x/jx-helpers/pkg/kube"
	"github.com/jenkins-x/jx-helpers/pkg/kube/jxclient"
	"github.com/jenkins-x/jx-helpers/pkg/options"
	"github.com/jenkins-x/jx-helpers/pkg/stringhelpers"
	"github.com/jenkins-x/jx-helpers/pkg/termcolor"
	"github.com/jenkins-x/jx-kube-client/pkg/kubeclient"
	"github.com/jenkins-x/jx-logging/pkg/log"
	"github.com/jenkins-x/jx-pipeline/pkg/tektonlog"
	"github.com/jenkins-x/jx/v2/pkg/tekton"
	"github.com/pkg/errors"
	tektonclient "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	"k8s.io/client-go/kubernetes"

	"github.com/spf13/cobra"

	gojenkins "github.com/jenkins-x/golang-jenkins"
	pipelineapi "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jenkins-x/jx-helpers/pkg/cobras/templates"
	"github.com/jenkins-x/jx/v2/pkg/cmd/opts"
)

// StopPipelineOptions contains the command line options
type Options struct {
	options.BaseOptions

	Args            []string
	Build           int
	Filter          string
	Namespace       string
	JenkinsSelector opts.JenkinsSelectorOptions
	Input           input.Interface
	KubeClient      kubernetes.Interface
	JXClient        versioned.Interface
	TektonClient    tektonclient.Interface

	Jobs map[string]gojenkins.Job
}

var (
	stopPipelineLong = templates.LongDesc(`
		Stops the pipeline build.

`)

	stopPipelineExample = templates.Examples(`
		# Stop a pipeline
		jx pipeline stop foo/bar/master -b 2

		# Select the pipeline to stop
		jx pipeline stop
	`)
)

// NewCmdPipelineStop creates the command
func NewCmdPipelineStop() (*cobra.Command, *Options) {
	o := &Options{}

	cmd := &cobra.Command{
		Use:     "stop",
		Short:   "Stops one or more pipelines",
		Long:    stopPipelineLong,
		Example: stopPipelineExample,
		Aliases: []string{"kill"},
		Run: func(cmd *cobra.Command, args []string) {
			o.Args = args
			err := o.Run()
			helper.CheckErr(err)
		},
	}
	cmd.Flags().IntVarP(&o.Build, "build", "", 0, "The build number to stop")
	cmd.Flags().StringVarP(&o.Filter, "filter", "f", "",
		"Filters all the available jobs by those that contain the given text")
	o.JenkinsSelector.AddFlags(cmd)

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

	return o.cancelPipelineRun()
}

func (o *Options) cancelPipelineRun() error {
	tektonClient := o.TektonClient
	ns := o.Namespace
	pipelines := tektonClient.TektonV1beta1().PipelineRuns(ns)
	prList, err := pipelines.List(metav1.ListOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to list PipelineRuns in namespace %s", ns)
	}

	if len(prList.Items) == 0 {
		return errors.Wrapf(err, "no PipelineRuns were found in namespace %s", ns)
	}
	var allNames []string
	m := map[string]*pipelineapi.PipelineRun{}
	for k := range prList.Items {
		pr := prList.Items[k]
		if tektonlog.PipelineRunIsComplete(&pr) {
			continue
		}
		labels := pr.Labels
		if labels == nil {
			continue
		}
		owner := labels[tekton.LabelOwner]
		repo := labels[tekton.LabelRepo]
		branch := labels[tekton.LabelBranch]
		context := labels[tekton.LabelContext]
		buildNumber := labels[tekton.LabelBuild]

		if owner == "" {
			log.Logger().Warnf("missing label %s on PipelineRun %s has labels %#v", tekton.LabelOwner,
				pr.Name, labels)
			continue
		}
		if repo == "" {
			log.Logger().Warnf("missing label %s on PipelineRun %s has labels %#v", tekton.LabelRepo,
				pr.Name, labels)
			continue
		}
		if branch == "" {
			log.Logger().Warnf("missing label %s on PipelineRun %s has labels %#v", tekton.LabelBranch,
				pr.Name, labels)
			continue
		}

		name := fmt.Sprintf("%s/%s/%s #%s", owner, repo, branch, buildNumber)

		if context != "" {
			name = fmt.Sprintf("%s-%s", name, context)
		}
		allNames = append(allNames, name)
		m[name] = &pr
	}
	sort.Strings(allNames)
	names := allNames
	if o.Filter != "" {
		names = stringhelpers.StringsContaining(allNames, o.Filter)
		if len(names) == 0 {
			log.Logger().Warnf("no PipelineRuns are still running which match the filter %s from all"+
				" possible names %s", o.Filter, strings.Join(allNames, ", "))
			return nil
		}
	}

	args := o.Args
	if len(args) == 0 {
		var name string
		name, err = o.Input.PickNameWithDefault(names, "Which pipeline do you want to stop: ",
			name, "select a pipeline to cancel")
		if err != nil {
			return err
		}

		var answer bool
		if answer, err = o.Input.Confirm(fmt.Sprintf("cancel pipeline %s", name), true,
			"you can always restart a cancelled pipeline with 'jx start pipeline'"); !answer {
			return err
		}
		args = []string{name}
	}
	for _, a := range args {
		pr := m[a]
		if pr == nil {
			return fmt.Errorf("no PipelineRun found for name %s", a)
		}
		prName := pr.Name
		pr, err = pipelines.Get(prName, metav1.GetOptions{})
		if err != nil {
			return errors.Wrapf(err, "getting PipelineRun %s", prName)
		}
		if tektonlog.PipelineRunIsComplete(pr) {
			log.Logger().Infof("PipelineRun %s has already completed", termcolor.ColorInfo(prName))
			continue
		}
		err = tektonlog.CancelPipelineRun(tektonClient, ns, pr)
		if err != nil {
			return errors.Wrapf(err, "failed to cancel pipeline %s in namespace %s", prName, ns)
		}
		log.Logger().Infof("cancelled PipelineRun %s", termcolor.ColorInfo(prName))
	}
	return nil
}
