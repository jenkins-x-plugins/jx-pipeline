package get

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/jenkins-x-plugins/jx-pipeline/pkg/pipelines"

	"github.com/jenkins-x-plugins/jx-pipeline/pkg/constants"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/tektonlog"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/triggers"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube"
	"github.com/jenkins-x/jx-helpers/v3/pkg/options"
	"github.com/jenkins-x/jx-helpers/v3/pkg/outputformat"
	"github.com/jenkins-x/jx-helpers/v3/pkg/table"
	"github.com/jenkins-x/jx-kube-client/v3/pkg/kubeclient"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	pipelineapi "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	tektonclient "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/templates"
)

// Options is the start of the data required to perform the operation.
// As new fields are added, add them here instead of
// referencing the cmd.Flags()
type Options struct {
	options.BaseOptions

	KubeClient          kubernetes.Interface
	TektonClient        tektonclient.Interface
	Format              string
	Namespace           string
	LighthouseConfigMap string
	ViewPostsubmits     bool
	ViewPresubmits      bool
}

var (
	cmdLong = templates.LongDesc(`
		Display one or more pipelines.

`)

	cmdExample = templates.Examples(`
		# list all pipelines
		jx pipeline get
	`)
)

// NewCmdPipelineGet creates the command
func NewCmdPipelineGet() (*cobra.Command, *Options) {
	o := &Options{}

	cmd := &cobra.Command{
		Use:     "get",
		Short:   "Display one or more pipelines",
		Long:    cmdLong,
		Example: cmdExample,
		Aliases: []string{"list", "ls"},
		Run: func(cmd *cobra.Command, args []string) {
			err := o.Run()
			helper.CheckErr(err)
		},
	}

	cmd.Flags().StringVarP(&o.Format, "format", "f", "", "The output format such as 'yaml' or 'json'")
	cmd.Flags().StringVarP(&o.Namespace, "namespace", "n", "", "The kubernetes namespace to use. If not specified the default namespace is used")
	cmd.Flags().StringVarP(&o.LighthouseConfigMap, "configmap", "", constants.LighthouseConfigMapName, "The name of the Lighthouse ConfigMap to find the trigger configurations")
	cmd.Flags().BoolVarP(&o.ViewPostsubmits, "postsubmit", "", false, "Views the available lighthouse postsubmit triggers rather than just the current PipelineRuns")
	cmd.Flags().BoolVarP(&o.ViewPresubmits, "presubmit", "", false, "Views the available lighthouse presubmit triggers rather than just the current PipelineRuns")

	o.BaseOptions.AddBaseFlags(cmd)
	return cmd, o
}

// Validate verifies things are setup correctly
func (o *Options) Validate() error {
	var err error
	o.KubeClient, o.Namespace, err = kube.LazyCreateKubeClientAndNamespace(o.KubeClient, o.Namespace)
	if err != nil {
		return errors.Wrapf(err, "failed to create kube client")
	}

	if o.TektonClient == nil {
		f := kubeclient.NewFactory()
		cfg, err := f.CreateKubeConfig()
		if err != nil {
			return errors.Wrap(err, "failed to get kubernetes config")
		}
		o.TektonClient, err = tektonclient.NewForConfig(cfg)
		if err != nil {
			return errors.Wrap(err, "error building tekton client")
		}
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

	if o.ViewPresubmits {
		return o.renderPresubmits(ctx)
	}
	if o.ViewPostsubmits {
		return o.renderPostsubmits(ctx)
	}
	return o.renderPipelineRuns(ctx)
}

// renderPostsubmits renders the current Lighthouse postsubmit triggers
func (o *Options) renderPostsubmits(ctx context.Context) error {
	cfg, err := triggers.LoadLighthouseConfig(ctx, o.KubeClient, o.Namespace, o.LighthouseConfigMap, false)
	if err != nil {
		return errors.Wrapf(err, "failed to load lighthouse config")
	}

	out := os.Stdout
	t := table.CreateTable(out)
	t.AddRow("REPOSITORY", "NAME", "BRANCHES")

	for repo, submits := range cfg.Postsubmits {
		for i := range submits {
			submit := &submits[i]
			t.AddRow(repo, submit.Name, strings.Join(submit.Branches, ", "))
		}
	}
	t.Render()
	return nil
}

// renderPresubmits renders the current Lighthouse presubmit triggers
func (o *Options) renderPresubmits(ctx context.Context) error {
	cfg, err := triggers.LoadLighthouseConfig(ctx, o.KubeClient, o.Namespace, o.LighthouseConfigMap, false)
	if err != nil {
		return errors.Wrapf(err, "failed to load lighthouse config")
	}

	out := os.Stdout
	t := table.CreateTable(out)
	t.AddRow("REPOSITORY", "CONTEXT", "RERUN COMMAND")

	for repo, submits := range cfg.Presubmits {
		for i := range submits {
			submit := &submits[i]
			t.AddRow(repo, submit.Name, submit.RerunCommand)
		}
	}
	t.Render()
	return nil
}

// renderPipelines view the current tekton PipelineRuns
func (o *Options) renderPipelineRuns(ctx context.Context) error {
	ns := o.Namespace
	tektonClient := o.TektonClient

	pipelineRuns := tektonClient.TektonV1beta1().PipelineRuns(ns)
	prList, err := pipelineRuns.List(ctx, metav1.ListOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to list PipelineRuns in namespace %s", ns)
	}

	if len(prList.Items) == 0 {
		return errors.New(fmt.Sprintf("no PipelineRuns were found in namespace %s", ns))
	}

	var owner, repo, branch, triggerContext, buildNumber, status string
	var names []string
	m := map[string]*pipelineapi.PipelineRun{}
	for k := range prList.Items {
		pr := prList.Items[k]
		status = "not completed"
		if tektonlog.PipelineRunIsComplete(&pr) {
			status = "completed"
		}
		labels := pr.Labels
		if labels == nil {
			continue
		}

		owner = pipelines.GetLabel(labels, pipelines.OwnerLabels)
		repo = pipelines.GetLabel(labels, pipelines.RepoLabels)
		branch = pipelines.GetLabel(labels, pipelines.BranchLabels)
		triggerContext = pipelines.GetLabel(labels, pipelines.ContextLabels)
		buildNumber = pipelines.GetLabel(labels, pipelines.BuildLabels)

		if owner == "" {
			log.Logger().Warnf("missing label %s on PipelineRun %s has labels %#v", tektonlog.LabelOwner, pr.Name, labels)
			continue
		}
		if repo == "" {
			log.Logger().Warnf("missing label %s on PipelineRun %s has labels %#v", tektonlog.LabelRepo, pr.Name, labels)
			continue
		}
		if branch == "" {
			log.Logger().Warnf("missing label %s on PipelineRun %s has labels %#v", tektonlog.LabelBranch, pr.Name, labels)
			continue
		}

		name := fmt.Sprintf("%s/%s/%s #%s %s", owner, repo, branch, buildNumber, status)

		if triggerContext != "" {
			name = fmt.Sprintf("%s-%s", name, triggerContext)
		}
		names = append(names, name)
		m[name] = &pr
	}

	sort.Strings(names)

	out := os.Stdout
	if o.Format != "" {
		return outputformat.Marshal(names, out, o.Format)
	}

	t := table.CreateTable(out)
	t.AddRow("Name", "URL", "LAST_BUILD", "STATUS", "DURATION")

	for _, j := range names {
		t.AddRow(j, "N/A", "N/A", "N/A", "N/A")
	}
	t.Render()
	return nil
}
