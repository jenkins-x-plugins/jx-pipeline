package breakpoint

import (
	"fmt"
	"os"
	"strings"

	"github.com/jenkins-x-plugins/jx-pipeline/pkg/lighthouses"
	v1 "github.com/jenkins-x/jx-api/v4/pkg/apis/jenkins.io/v1"
	"github.com/jenkins-x/jx-helpers/v3/pkg/input"
	"github.com/jenkins-x/jx-helpers/v3/pkg/input/inputfactory"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/jxclient"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/jxenv"
	"github.com/jenkins-x/jx-helpers/v3/pkg/stringhelpers"
	"github.com/jenkins-x/jx-helpers/v3/pkg/termcolor"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
	"github.com/jenkins-x/lighthouse-client/pkg/apis/lighthouse"
	"github.com/jenkins-x/lighthouse-client/pkg/apis/lighthouse/v1alpha1"
	lhclient "github.com/jenkins-x/lighthouse-client/pkg/client/clientset/versioned"
	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/jenkins-x/jx-api/v4/pkg/client/clientset/versioned"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube"
	"github.com/jenkins-x/jx-helpers/v3/pkg/options"

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

	BreakpointNames []string
	KubeClient      kubernetes.Interface
	JXClient        versioned.Interface
	LHClient        lhclient.Interface
	Namespace       string
	Input           input.Interface
	Breakpoints     []*v1alpha1.LighthouseBreakpoint
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
		Run: func(_ *cobra.Command, _ []string) {
			err := o.Run()
			helper.CheckErr(err)
		},
	}

	cmd.Flags().StringVarP(&o.Namespace, "namespace", "n", "", "The kubernetes namespace to use. If not specified the default namespace is used")
	cmd.Flags().StringArrayVarP(&o.BreakpointNames, "breakpoints", "p", []string{"onFailure"}, "The breakpoint names to use when creating a new breakpoint")

	o.BaseOptions.AddBaseFlags(cmd)
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
	if o.Out == nil {
		o.Out = os.Stdout
	}
	return nil
}

// Run implements this command
func (o *Options) Run() error {
	err := o.Validate()
	if err != nil {
		return fmt.Errorf("failed to validate options: %w", err)
	}

	ns, _, err := jxenv.GetDevNamespace(o.KubeClient, o.Namespace)
	if err != nil {
		return fmt.Errorf("failed to find dev namespace: %w", err)
	}
	jxClient := o.JXClient

	ctx := o.GetContext()
	bpList, err := o.LHClient.LighthouseV1alpha1().LighthouseBreakpoints(ns).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("could not list LighthouseBreakpoint resources: %w", err)
	}
	for i := range bpList.Items {
		b := &bpList.Items[i]
		o.Breakpoints = append(o.Breakpoints, b)
	}

	list, err := jxClient.JenkinsV1().PipelineActivities(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	items := list.Items

	var names []string
	m := map[string]*v1.PipelineActivity{}

	for i := range items {
		a := &items[i]

		label := o.ToLabel(a)
		if stringhelpers.StringArrayIndex(names, label) < 0 {
			names = append(names, label)
		}
		m[label] = a
	}

	name, err := o.Input.PickNameWithDefault(names, "Pick the pipeline: ", "", "Please select a pipeline to add/edit/remove the breakpoint")
	if err != nil {
		return fmt.Errorf("failed to select pipeline: %w", err)
	}
	if name == "" {
		return fmt.Errorf("no pipeline selected")
	}
	pa := m[name]
	if pa == nil {
		return fmt.Errorf("no PipelineActivity for: %s select pipeline", name)
	}

	log.Logger().Infof("selected pipeline: %s", info(name))

	f := ToBreakpointFilter(pa)

	for _, bp := range o.Breakpoints {
		if bp.Spec.Filter.Matches(f) { //nolint:gocritic
			// lets confirm the deletion of the breakpoint?
			confirm, inputErr := o.Input.Confirm("would you like to remove Breakpoint "+bp.Name, false, "confirm if you would like to delete the LighthouseBreakpoint resource")
			if inputErr != nil {
				return fmt.Errorf("failed to confirm deletion: %w", inputErr)
			}
			if !confirm {
				return nil
			}
			err = o.LHClient.LighthouseV1alpha1().LighthouseBreakpoints(ns).Delete(ctx, bp.Name, metav1.DeleteOptions{})
			if err != nil {
				return fmt.Errorf("failed to delete the LighthouseBreakpoint %s: %w", bp.Name, err)
			}
			log.Logger().Infof("deleted the LighthouseBreakpoint %s", info(bp.Name))
			return nil
		}
	}

	// lets create a new Breakpoint for this filter
	bp := &v1alpha1.LighthouseBreakpoint{
		TypeMeta: metav1.TypeMeta{
			Kind:       "LighthouseBreakpoint",
			APIVersion: lighthouse.GroupAndVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      pa.Name,
			Namespace: ns,
		},
		Spec: v1alpha1.LighthouseBreakpointSpec{
			Filter: *f,
			Debug: pipelinev1.TaskRunDebug{
				Breakpoints: &pipelinev1.TaskBreakpoints{
					BeforeSteps: o.BreakpointNames,
				},
			},
		},
	}
	_, err = o.LHClient.LighthouseV1alpha1().LighthouseBreakpoints(ns).Create(ctx, bp, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create the LighthouseBreakpoint %#v: %w", bp, err)
	}
	log.Logger().Infof("created the LighthouseBreakpoint %s", info(bp.Name))
	return nil
}

func (o *Options) ToLabel(a *v1.PipelineActivity) string {
	as := &a.Spec
	repo := as.GitOwner + "/" + as.GitRepository
	label := repo + " " + as.GitBranch + " " + as.Context

	f := ToBreakpointFilter(a)

	debug := f.ResolveDebug(o.Breakpoints)
	if debug != nil {
		label += " => breakpoint: " + strings.Join(debug.Breakpoints.BeforeSteps, ", ")
	}
	return label
}

// ToBreakpointFilter converts the PipelineActivity to a filter for breakpoints
func ToBreakpointFilter(a *v1.PipelineActivity) *v1alpha1.LighthousePipelineFilter {
	as := &a.Spec
	return &v1alpha1.LighthousePipelineFilter{
		Owner:      as.GitOwner,
		Repository: as.GitRepository,
		Branch:     as.GitBranch,
		Context:    as.Context,
	}
}
