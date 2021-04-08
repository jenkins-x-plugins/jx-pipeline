package stop

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jenkins-x-plugins/jx-pipeline/pkg/lighthouses"

	"github.com/jenkins-x-plugins/jx-pipeline/pkg/pipelines"

	"github.com/jenkins-x-plugins/jx-pipeline/pkg/tektonlog"
	"github.com/jenkins-x/jx-api/v4/pkg/client/clientset/versioned"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/v3/pkg/input"
	"github.com/jenkins-x/jx-helpers/v3/pkg/input/inputfactory"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/jxclient"
	"github.com/jenkins-x/jx-helpers/v3/pkg/options"
	"github.com/jenkins-x/jx-helpers/v3/pkg/stringhelpers"
	"github.com/jenkins-x/jx-kube-client/v3/pkg/kubeclient"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	tektonclient "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	"k8s.io/client-go/kubernetes"

	pipelineapi "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/templates"
)

// StopPipelineOptions contains the command line options
type Options struct {
	options.BaseOptions

	Args         []string
	Build        int
	Filter       string
	Namespace    string
	CatalogSHA   string
	Input        input.Interface
	KubeClient   kubernetes.Interface
	JXClient     versioned.Interface
	TektonClient tektonclient.Interface
}

var (
	cmdLong = templates.LongDesc(`
		Stops the pipeline build.

`)

	cmdExample = templates.Examples(`
		# Select the pipeline to stop
		jx pipeline stop

		# Stop a pipeline with a filter
		jx pipeline stop -f myapp -n 2

		# Stop a pipeline for a specific org/repo/branch
		jx pipeline stop myorg/myrepo/main
	`)
)

// NewCmdPipelineStop creates the command
func NewCmdPipelineStop() (*cobra.Command, *Options) {
	o := &Options{}

	cmd := &cobra.Command{
		Use:     "stop",
		Short:   "Stops one or more pipelines",
		Long:    cmdLong,
		Example: cmdExample,
		Aliases: []string{"kill"},
		Run: func(cmd *cobra.Command, args []string) {
			o.Args = args
			err := o.Run()
			helper.CheckErr(err)
		},
	}
	cmd.Flags().IntVarP(&o.Build, "build", "n", 0, "The build number to stop")
	cmd.Flags().StringVarP(&o.Filter, "filter", "f", "",
		"Filters all the available jobs by those that contain the given text")

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
	lighthouses.DefaultPipelineCatalogSHA(o.CatalogSHA)
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
	ctx := o.GetContext()
	tektonClient := o.TektonClient
	ns := o.Namespace
	pipelineRuns := tektonClient.TektonV1beta1().PipelineRuns(ns)
	prList, err := pipelineRuns.List(ctx, metav1.ListOptions{})
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
		owner := pipelines.GetLabel(labels, pipelines.OwnerLabels)
		repo := pipelines.GetLabel(labels, pipelines.RepoLabels)
		branch := pipelines.GetLabel(labels, pipelines.BranchLabels)
		triggerContext := pipelines.GetLabel(labels, pipelines.ContextLabels)
		buildNumber := pipelines.GetLabel(labels, pipelines.BuildLabels)

		if owner == "" {
			log.Logger().Warnf("missing label %s on PipelineRun %s has labels %#v", tektonlog.LabelOwner,
				pr.Name, labels)
			continue
		}
		if repo == "" {
			log.Logger().Warnf("missing label %s on PipelineRun %s has labels %#v", tektonlog.LabelRepo,
				pr.Name, labels)
			continue
		}
		if branch == "" {
			log.Logger().Warnf("missing label %s on PipelineRun %s has labels %#v", tektonlog.LabelBranch,
				pr.Name, labels)
			continue
		}

		name := fmt.Sprintf("%s/%s/%s #%s", owner, repo, branch, buildNumber)

		if triggerContext != "" {
			name = fmt.Sprintf("%s-%s", name, triggerContext)
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
	if len(args) > 0 {
		var filteredNames []string
		for _, a := range args {
			for _, name := range names {
				if strings.Contains(name, a) && stringhelpers.StringArrayIndex(filteredNames, name) < 0 {
					filteredNames = append(filteredNames, name)
				}
			}
		}
		sort.Strings(filteredNames)
		names = filteredNames
	}

	var name string
	name, err = o.Input.PickNameWithDefault(names, "Which pipeline do you want to stop: ",
		name, "select a pipeline to cancel")
	if err != nil {
		return err
	}

	if len(names) == 0 {
		log.Logger().Infof("no running pipelines available to stop")
		return nil
	}
	var answer bool
	if answer, err = o.Input.Confirm(fmt.Sprintf("cancel pipeline %s", name), true,
		"you can always restart a cancelled pipeline with 'jx start pipeline'"); !answer {
		return err
	}
	args = []string{name}

	return nil
}
