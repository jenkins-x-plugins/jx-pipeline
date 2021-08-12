package breakpoint

import (
	v1 "github.com/jenkins-x/jx-api/v4/pkg/apis/jenkins.io/v1"
	"github.com/jenkins-x/jx-helpers/v3/pkg/input"
	"github.com/jenkins-x/jx-helpers/v3/pkg/input/inputfactory"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/jxclient"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/jxenv"
	"github.com/jenkins-x/jx-helpers/v3/pkg/termcolor"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
	"os"

	"github.com/jenkins-x/jx-api/v4/pkg/client/clientset/versioned"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube"
	"github.com/jenkins-x/jx-helpers/v3/pkg/options"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/templates"
)

// Options is the start of the data required to perform the operation.
// As new fields are added, add them here instead of
// referencing the cmd.Flags()
type Options struct {
	options.BaseOptions

	KubeClient kubernetes.Interface
	JXClient   versioned.Interface
	Namespace  string
	Input      input.Interface
}

var (
	info = termcolor.ColorInfo

	cmdLong = templates.LongDesc(`
		Add or remove pipeline breakpoints for debugging pipeline steps.

`)

	cmdExample = templates.Examples(`
		# add or remove a breakpoint
		jx pipeline breakpoint
	`)
)

// NewCmdPipelineBreakpoint creates the command
func NewCmdPipelineBreakpoint() (*cobra.Command, *Options) {
	o := &Options{}

	cmd := &cobra.Command{
		Use:     "debug",
		Short:   "Add or remove pipeline breakpoints for debugging pipeline steps",
		Long:    cmdLong,
		Example: cmdExample,
		Aliases: []string{"bp", "breakpoint"},
		Run: func(cmd *cobra.Command, args []string) {
			err := o.Run()
			helper.CheckErr(err)
		},
	}

	cmd.Flags().StringVarP(&o.Namespace, "namespace", "n", "", "The kubernetes namespace to use. If not specified the default namespace is used")

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
	o.JXClient, err = jxclient.LazyCreateJXClient(o.JXClient)
	if err != nil {
		return errors.Wrapf(err, "failed to create the jx client")
	}

	if o.Input == nil {
		o.Input = inputfactory.NewInput(&o.BaseOptions)
	}
	if o.Out == nil {
		o.Out = os.Stdout
	}
	return nil
}

// Run implements this command
func (o *Options) Run() error {
	err := o.Validate()
	if err != nil {
		return errors.Wrapf(err, "failed to validate options")
	}

	ns, _, err := jxenv.GetDevNamespace(o.KubeClient, o.Namespace)
	if err != nil {
		return errors.Wrapf(err, "failed to find dev namespace")
	}
	jxClient := o.JXClient

	ctx := o.GetContext()
	list, err := jxClient.JenkinsV1().PipelineActivities(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	items := list.Items

	var names []string
	m := map[string]*v1.PipelineActivity{}

	for i := range items {
		a := &items[i]

		label := ToLabel(a)
		names = append(names, label)
		m[label] = a
	}

	name, err := o.Input.PickNameWithDefault(names, "Pick the pipeline: ", "", "Please select a pipeline to add/edit/remove the breakpoint")
	if err != nil {
		return errors.Wrapf(err, "failed to select pipeline")
	}
	if name == "" {
		return errors.Errorf("no pipeline selected")
	}
	pa := m[name]
	if pa == nil {
		return errors.Wrapf(err, "no PipelineActivity for: %s select pipeline", name)
	}

	log.Logger().Infof("selected pipeline: %s", info(name))
	return nil
}

func ToLabel(a *v1.PipelineActivity) string {
	as := &a.Spec
	repo := as.GitOwner + "/" + as.GitRepository
	return repo + " " + as.GitBranch + " #" + as.Build + " " + as.Context
}
