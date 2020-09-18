package stop

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jenkins-x/jx-logging/pkg/log"
	"github.com/jenkins-x/jx/v2/pkg/cmd/get"
	"github.com/jenkins-x/jx/v2/pkg/cmd/helper"
	"github.com/jenkins-x/jx/v2/pkg/tekton"
	"github.com/pkg/errors"

	"github.com/spf13/cobra"

	gojenkins "github.com/jenkins-x/golang-jenkins"
	pipelineapi "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jenkins-x/jx/v2/pkg/cmd/opts"
	"github.com/jenkins-x/jx/v2/pkg/cmd/templates"
	"github.com/jenkins-x/jx/v2/pkg/util"
)

// StopPipelineOptions contains the command line options
type Options struct {
	get.Options

	Build           int
	Filter          string
	JenkinsSelector opts.JenkinsSelectorOptions

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
func NewCmdPipelineStop(commonOpts *opts.CommonOptions) (*cobra.Command, *Options) {
	o := &Options{
		Options: get.Options{
			CommonOptions: commonOpts,
		},
	}

	cmd := &cobra.Command{
		Use:     "stop",
		Short:   "Stops one or more pipelines",
		Long:    stopPipelineLong,
		Example: stopPipelineExample,
		Aliases: []string{"kill"},
		Run: func(cmd *cobra.Command, args []string) {
			o.Cmd = cmd
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

// Run implements this command
func (o *Options) Run() error {
	devEnv, _, err := o.DevEnvAndTeamSettings()
	if err != nil {
		return err
	}

	isProw := devEnv.Spec.IsProwOrLighthouse()
	if !isProw {
		return errors.New("Only prow/lighthouse is supported as a webhook engine")
	}
	return o.cancelPipelineRun()
}

func (o *Options) cancelPipelineRun() error {
	tektonClient, ns, err := o.TektonClient()
	if err != nil {
		return errors.Wrap(err, "could not create tekton client")
	}
	pipelines := tektonClient.TektonV1alpha1().PipelineRuns(ns)
	prList, err := pipelines.List(metav1.ListOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to list PipelineRuns in namespace %s", ns)
	}

	if len(prList.Items) == 0 {
		return errors.Wrapf(err, "no PipelineRuns were found in namespace %s", ns)
	}
	allNames := []string{}
	m := map[string]*pipelineapi.PipelineRun{}
	for k := range prList.Items {
		pr := prList.Items[k]
		if tekton.PipelineRunIsComplete(&pr) {
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
		names = util.StringsContaining(allNames, o.Filter)
		if len(names) == 0 {
			log.Logger().Warnf("no PipelineRuns are still running which match the filter %s from all"+
				" possible names %s", o.Filter, strings.Join(allNames, ", "))
			return nil
		}
	}

	args := o.Args
	if len(args) == 0 {
		name, err := util.PickName(names, "Which pipeline do you want to stop: ",
			"select a pipeline to cancel", o.GetIOFileHandles())
		if err != nil {
			return err
		}

		if answer, err := util.Confirm(fmt.Sprintf("cancel pipeline %s", name), true,
			"you can always restart a cancelled pipeline with 'jx start pipeline'",
			o.GetIOFileHandles()); !answer {
			return err
		}
		args = []string{name}
	}
	for _, a := range args {
		pr := m[a]
		if pr == nil {
			return fmt.Errorf("no PipelineRun found for name %s", a)
		}
		pr, err = pipelines.Get(pr.Name, metav1.GetOptions{})
		if err != nil {
			return errors.Wrapf(err, "getting PipelineRun %s", pr.Name)
		}
		if tekton.PipelineRunIsComplete(pr) {
			log.Logger().Infof("PipelineRun %s has already completed", util.ColorInfo(pr.Name))
			continue
		}
		err = tekton.CancelPipelineRun(tektonClient, ns, pr)
		if err != nil {
			return errors.Wrapf(err, "failed to cancel pipeline %s in namespace %s", pr.Name, ns)
		}
		log.Logger().Infof("cancelled PipelineRun %s", util.ColorInfo(pr.Name))
	}
	return nil
}
