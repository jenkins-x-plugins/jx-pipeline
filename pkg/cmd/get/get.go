package get

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/jenkins-x/jx-helpers/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/pkg/kube"
	"github.com/jenkins-x/jx-helpers/pkg/outputformat"
	"github.com/jenkins-x/jx-helpers/pkg/table"
	"github.com/jenkins-x/jx-kube-client/pkg/kubeclient"
	"github.com/jenkins-x/jx-logging/pkg/log"
	"github.com/jenkins-x/jx-pipeline/pkg/triggers"
	"github.com/jenkins-x/jx/v2/pkg/tekton"
	"github.com/pkg/errors"
	pipelineapi "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	tektonclient "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/spf13/cobra"

	"github.com/jenkins-x/jx/v2/pkg/cmd/templates"
)

// PipelineOptions is the start of the data required to perform the operation.
// As new fields are added, add them here instead of
// referencing the cmd.Flags()
type Options struct {
	KubeClient          kubernetes.Interface
	TektonClient        tektonclient.Interface
	Format              string
	Namespace           string
	LighthouseConfigMap string
	ViewPostsubmits     bool
	ViewPresubmits      bool
}

var (
	getPipelineLong = templates.LongDesc(`
		Display one or more pipelines.

`)

	getPipelineExample = templates.Examples(`
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
		Long:    getPipelineLong,
		Example: getPipelineExample,
		Aliases: []string{"list", "ls"},
		Run: func(cmd *cobra.Command, args []string) {
			err := o.Run()
			helper.CheckErr(err)
		},
	}

	cmd.Flags().StringVarP(&o.Format, "format", "f", "", "The output format such as 'yaml' or 'json'")
	cmd.Flags().StringVarP(&o.Namespace, "namespace", "n", "", "The kubernetes namespace to use. If not specified the default namespace is used")
	cmd.Flags().StringVarP(&o.LighthouseConfigMap, "configmap", "", "config", "The name of the Lighthouse ConfigMap to find the trigger configurations")
	cmd.Flags().BoolVarP(&o.ViewPostsubmits, "postsubmit", "", false, "Views the available lighthouse postsubmit triggers rather than just the current PipelineRuns")
	cmd.Flags().BoolVarP(&o.ViewPresubmits, "presubmit", "", false, "Views the available lighthouse presubmit triggers rather than just the current PipelineRuns")
	return cmd, o
}

// Validate verifies things are setup correctly
func (o *Options) Validate() error {
	var err error
	o.KubeClient, o.Namespace, err = kube.LazyCreateKubeClientAndNamespace(o.KubeClient, o.Namespace)
	if err != nil {
		return errors.Wrapf(err, "failed to create kube client")
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
	return nil
}

// Run implements this command
func (o *Options) Run() error {
	err := o.Validate()
	if err != nil {
		return errors.Wrapf(err, "failed to validate options")
	}

	if o.ViewPresubmits {
		return o.renderPresubmits()
	}
	if o.ViewPostsubmits {
		return o.renderPostsubmits()
	}
	return o.renderPipelineRuns()
}

// renderPostsubmits renders the current Lighthouse postsubmit triggers
func (o *Options) renderPostsubmits() error {
	cfg, err := triggers.LoadLighthouseConfig(o.KubeClient, o.Namespace, o.LighthouseConfigMap)
	if err != nil {
		return errors.Wrapf(err, "failed to load lighthouse config")
	}

	out := os.Stdout
	t := table.CreateTable(out)
	t.AddRow("REPOSITORY", "NAME", "BRANCHES")

	for repo, submits := range cfg.Postsubmits {
		for _, submit := range submits {
			t.AddRow(repo, submit.Name, strings.Join(submit.Branches, ", "))
		}
	}
	t.Render()
	return nil
}

// renderPresubmits renders the current Lighthouse presubmit triggers
func (o *Options) renderPresubmits() error {
	cfg, err := triggers.LoadLighthouseConfig(o.KubeClient, o.Namespace, o.LighthouseConfigMap)
	if err != nil {
		return errors.Wrapf(err, "failed to load lighthouse config")
	}

	out := os.Stdout
	t := table.CreateTable(out)
	t.AddRow("REPOSITORY", "CONTEXT", "RERUN COMMAND")

	for repo, submits := range cfg.Presubmits {
		for _, submit := range submits {
			t.AddRow(repo, submit.Name, submit.RerunCommand)
		}
	}
	t.Render()
	return nil
}

// renderPipelines view the current tekton PipelineRuns
func (o *Options) renderPipelineRuns() error {
	ns := o.Namespace
	tektonClient := o.TektonClient

	pipelines := tektonClient.TektonV1alpha1().PipelineRuns(ns)
	prList, err := pipelines.List(metav1.ListOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to list PipelineRuns in namespace %s", ns)
	}

	if len(prList.Items) == 0 {
		return errors.New(fmt.Sprintf("no PipelineRuns were found in namespace %s", ns))
	}

	var owner, repo, branch, context, buildNumber, status string
	names := []string{}
	m := map[string]*pipelineapi.PipelineRun{}
	for k := range prList.Items {
		pr := prList.Items[k]
		status = "not completed"
		if tekton.PipelineRunIsComplete(&pr) {
			status = "completed"
		}
		labels := pr.Labels
		if labels == nil {
			continue
		}
		owner = labels[tekton.LabelOwner]
		repo = labels[tekton.LabelRepo]
		branch = labels[tekton.LabelBranch]
		context = labels[tekton.LabelContext]
		buildNumber = labels[tekton.LabelBuild]

		if owner == "" {
			log.Logger().Warnf("missing label %s on PipelineRun %s has labels %#v", tekton.LabelOwner, pr.Name, labels)
			continue
		}
		if repo == "" {
			log.Logger().Warnf("missing label %s on PipelineRun %s has labels %#v", tekton.LabelRepo, pr.Name, labels)
			continue
		}
		if branch == "" {
			log.Logger().Warnf("missing label %s on PipelineRun %s has labels %#v", tekton.LabelBranch, pr.Name, labels)
			continue
		}

		name := fmt.Sprintf("%s/%s/%s #%s %s", owner, repo, branch, buildNumber, status)

		if context != "" {
			name = fmt.Sprintf("%s-%s", name, context)
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
