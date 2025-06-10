package stop

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/activities"

	v1 "github.com/jenkins-x/jx-api/v4/pkg/apis/jenkins.io/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/jenkins-x/jx-helpers/v3/pkg/termcolor"

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

	"github.com/spf13/cobra"
	tektonclient "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	"k8s.io/client-go/kubernetes"

	pipelineapi "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/templates"
)

// StopPipelineOptions contains the command line options
type Options struct {
	options.BaseOptions

	Args         []string
	Filter       string
	Build        string
	Branch       string
	Context      string
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

		# Stop a pipeline with a filter and a build number
		jx pipeline stop -f myapp -n 2

		# Stop a pipeline for a specific org/repo/branch
		jx pipeline stop myorg/myrepo/main

		# Stop a pipeline for a specific context and branch
		jx pipeline stop --context pr --branch PR-456
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
		Run: func(_ *cobra.Command, args []string) {
			o.Args = args
			err := o.Run()
			helper.CheckErr(err)
		},
	}
	o.BaseOptions.AddBaseFlags(cmd)

	cmd.Flags().StringVarP(&o.Branch, "branch", "r", "", "The branch to filter by")
	cmd.Flags().StringVarP(&o.Context, "context", "c", "", "The context to filter by")
	cmd.Flags().StringVarP(&o.Build, "build", "n", "", "The build number to stop")
	cmd.Flags().StringVarP(&o.Filter, "filter", "f", "",
		"Filters all the available pipeline names")

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
		return fmt.Errorf("failed to validate options: %w", err)
	}

	return o.cancelPipelineRun()
}

func (o *Options) cancelPipelineRun() error {
	ctx := o.GetContext()
	jxClient := o.JXClient
	tektonClient := o.TektonClient
	ns := o.Namespace
	pipelineRuns := tektonClient.TektonV1().PipelineRuns(ns)
	prList, err := pipelineRuns.List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to list PipelineRuns in namespace %s: %w", ns, err)
	}

	paList, err := jxClient.JenkinsV1().PipelineActivities(ns).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to list PipelineActivity resources in namespace %s: %w", ns, err)
	}
	if paList == nil {
		paList = &v1.PipelineActivityList{}
	}
	activityResolver := pipelines.NewActivityResolver(paList.Items)

	if len(prList.Items) == 0 {
		return fmt.Errorf("no PipelineRuns were found in namespace %s", ns)
	}
	var allNames []string
	m := map[string]*pipelineapi.PipelineRun{}
	if prList != nil {
		for k := range prList.Items {
			pr := &prList.Items[k]
			if tektonlog.PipelineRunIsComplete(pr) {
				continue
			}
			labels := pr.Labels
			if labels == nil {
				continue
			}
			owner := activities.GetLabel(labels, activities.OwnerLabels)
			repo := activities.GetLabel(labels, activities.RepoLabels)
			branch := activities.GetLabel(labels, activities.BranchLabels)
			triggerContext := activities.GetLabel(labels, activities.ContextLabels)
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

			pa, err := activityResolver.ToPipelineActivity(pr)
			if err != nil {
				log.Logger().Warnf("failed to get PipelineActivity: %v", err)
			}
			if pa == nil {
				continue
			}
			context := pa.Spec.Context
			if context == "" {
				context = triggerContext
			}
			build := pa.Spec.Build
			if build == "" {
				build = activities.GetLabel(labels, activities.BuildLabels)
			}
			if o.Build != "" && build != o.Build {
				continue
			}
			if o.Branch != "" && branch != o.Branch {
				continue
			}
			if o.Context != "" && context != o.Context {
				continue
			}
			name := fmt.Sprintf("%s/%s/%s %s #%s", owner, repo, branch, context, build)
			allNames = append(allNames, name)
			m[name] = pr
		}
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

	pr := m[name]
	if pr == nil {
		return fmt.Errorf("could not find PipelineRun %s", name)
	}
	prName := pr.Name
	err = tektonlog.CancelPipelineRun(ctx, tektonClient, ns, pr)
	if err != nil {
		return fmt.Errorf("failed to cancel pipeline %s in namespace %s: %w", prName, ns, err)
	}
	log.Logger().Infof("cancelled PipelineRun %s", termcolor.ColorInfo(prName))

	return nil
}
