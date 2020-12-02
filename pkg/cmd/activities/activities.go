package activities

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/ghodss/yaml"
	v1 "github.com/jenkins-x/jx-api/v4/pkg/apis/core/v4beta1"
	"github.com/jenkins-x/jx-api/v4/pkg/client/clientset/versioned"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/templates"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/activities"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/jxclient"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/jxenv"
	"github.com/jenkins-x/jx-helpers/v3/pkg/options"
	"github.com/jenkins-x/jx-helpers/v3/pkg/table"
	"github.com/jenkins-x/jx-helpers/v3/pkg/termcolor"
	"github.com/jenkins-x/jx-kube-client/v3/pkg/kubeclient"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	tektonclient "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"knative.dev/pkg/signals"
)

const (
	indentation = "  "
)

// Options containers the CLI options
type Options struct {
	options.BaseOptions

	Format       string
	Namespace    string
	Filter       string
	BuildNumber  string
	Watch        bool
	Sort         bool
	KubeClient   kubernetes.Interface
	JXClient     versioned.Interface
	TektonClient tektonclient.Interface
	Out          io.Writer
	Results      []v1.PipelineActivity
}

var (
	cmdLong = templates.LongDesc(`
		Display the current activities for one or more projects.
`)

	cmdExample = templates.Examples(`
		# List the current activities for all applications in the current team
		jx pipeline activities

		# List the current activities for application 'foo'
		jx pipeline act -f foo

		# Watch the activities for application 'foo'
		jx pipeline act -f foo -w
	`)
)

// NewCmdActivities creates the new command for: jx get version
func NewCmdActivities() (*cobra.Command, *Options) {
	o := &Options{}

	cmd := &cobra.Command{
		Use:     "activities",
		Short:   "Display one or more Activities on projects",
		Aliases: []string{"activity", "act"},
		Long:    cmdLong,
		Example: cmdExample,
		Run: func(cmd *cobra.Command, args []string) {
			err := o.Run()
			helper.CheckErr(err)
		},
	}
	cmd.Flags().StringVarP(&o.Filter, "filter", "f", "", "Text to filter the pipeline names")
	cmd.Flags().StringVarP(&o.BuildNumber, "build", "", "", "The build number to filter on")
	cmd.Flags().BoolVarP(&o.Watch, "watch", "w", false, "Whether to watch the activities for changes")
	cmd.Flags().BoolVarP(&o.Sort, "sort", "s", false, "Sort activities by timestamp")

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

	t := table.CreateTable(o.Out)
	t.SetColumnAlign(1, table.ALIGN_RIGHT)
	t.SetColumnAlign(2, table.ALIGN_RIGHT)
	t.AddRow("STEP", "STARTED AGO", "DURATION", "STATUS")

	if o.Watch {
		return o.WatchActivities(&t, jxClient, ns)
	}

	ctx := o.GetContext()
	list, err := jxClient.CoreV4beta1().PipelineActivities(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	items := list.Items
	if o.Sort {
		activities.SortActivities(items)
	}

	for i := range items {
		a := &items[i]
		o.addTableRow(&t, a)
	}
	t.Render()

	o.Results = items
	return nil
}

func (o *Options) addTableRow(t *table.Table, activity *v1.PipelineActivity) bool {
	if o.matches(activity) {
		spec := &activity.Spec
		text := ""
		version := activity.Spec.Version
		if version != "" {
			text = "Version: " + termcolor.ColorInfo(version)
		}
		statusText := statusString(activity.Spec.Status)
		if statusText == "" {
			statusText = text
		} else {
			statusText += " " + text
		}
		t.AddRow(spec.Pipeline+" #"+spec.Build,
			timeToString(spec.StartedTimestamp),
			DurationString(spec.StartedTimestamp, spec.CompletedTimestamp),
			statusText)
		indent := indentation
		for _, step := range spec.Steps {
			s := step
			o.addStepRow(t, &s, indent)
		}
		return true
	}
	return false
}

func (o *Options) WatchActivities(t *table.Table, jxClient versioned.Interface, ns string) error {
	yamlSpecMap := map[string]string{}
	activity := &v1.PipelineActivity{}

	listWatch := cache.NewListWatchFromClient(jxClient.CoreV4beta1().RESTClient(), "pipelineactivities", ns, fields.Everything())
	kube.SortListWatchByName(listWatch)
	_, controller := cache.NewInformer(
		listWatch,
		activity,
		time.Minute*10,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				o.onActivity(t, obj, yamlSpecMap)
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				o.onActivity(t, newObj, yamlSpecMap)
			},
			DeleteFunc: func(obj interface{}) {
			},
		},
	)

	stop := signals.SetupSignalHandler()
	go controller.Run(stop)

	// Wait forever
	select {}
}

func (o *Options) onActivity(t *table.Table, obj interface{}, yamlSpecMap map[string]string) {
	activity, ok := obj.(*v1.PipelineActivity)
	if !ok {
		log.Logger().Infof("Object is not a PipelineActivity %#v", obj)
		return
	}
	data, err := yaml.Marshal(&activity.Spec)
	if err != nil {
		log.Logger().Infof("Failed to marshal Activity.Spec to YAML: %s", err)
	} else {
		text := string(data)
		name := activity.Name
		old := yamlSpecMap[name]
		if old == "" || old != text {
			yamlSpecMap[name] = text
			if o.addTableRow(t, activity) {
				t.Render()
				t.Clear()
			}
		}
	}
}

func (o *Options) addStepRow(t *table.Table, parent *v1.PipelineActivityStep, indent string) {
	stage := parent.Stage
	preview := parent.Preview
	promote := parent.Promote
	if stage != nil {
		addStageRow(t, stage, indent)
	} else if preview != nil {
		addPreviewRow(t, preview, indent)
	} else if promote != nil {
		addPromoteRow(t, promote, indent)
	} else {
		log.Logger().Warnf("Unknown step kind %#v", parent)
	}
}

func addStageRow(t *table.Table, stage *v1.StageActivityStep, indent string) {
	name := "Stage"
	if stage.Name != "" {
		name = ""
	}
	addStepRowItem(t, &stage.CoreActivityStep, indent, name, "")

	indent += indentation
	for _, step := range stage.Steps {
		s := step
		addStepRowItem(t, &s, indent, "", "")
	}
}

func addPreviewRow(t *table.Table, parent *v1.PreviewActivityStep, indent string) {
	pullRequestURL := parent.PullRequestURL
	if pullRequestURL == "" {
		pullRequestURL = parent.Environment
	}
	addStepRowItem(t, &parent.CoreActivityStep, indent, "Preview", termcolor.ColorInfo(pullRequestURL))
	indent += indentation

	appURL := parent.ApplicationURL
	if appURL != "" {
		addStepRowItem(t, &parent.CoreActivityStep, indent, "Preview Application", termcolor.ColorInfo(appURL))
	}
}

func addPromoteRow(t *table.Table, parent *v1.PromoteActivityStep, indent string) {
	addStepRowItem(t, &parent.CoreActivityStep, indent, "Promote: "+parent.Environment, "")
	indent += indentation

	pullRequest := parent.PullRequest
	update := parent.Update
	if pullRequest != nil {
		addStepRowItem(t, &pullRequest.CoreActivityStep, indent, "PullRequest", describePromotePullRequest(pullRequest))
	}
	if update != nil {
		addStepRowItem(t, &update.CoreActivityStep, indent, "Update", describePromoteUpdate(update))

		if parent.ApplicationURL != "" {
			addStepRowItem(t, &update.CoreActivityStep, indent, "Promoted", " Application is at: "+termcolor.ColorInfo(parent.ApplicationURL))
		}
	}
}

func addStepRowItem(t *table.Table, step *v1.CoreActivityStep, indent, name, description string) {
	text := step.Description
	if description != "" {
		if text == "" {
			text = description
		} else {
			text += " " + description
		}
	}
	textName := step.Name
	if textName == "" {
		textName = name
	} else if name != "" {
		textName = name + ":" + textName
	}
	t.AddRow(indent+textName,
		timeToString(step.StartedTimestamp),
		DurationString(step.StartedTimestamp, step.CompletedTimestamp),
		statusString(step.Status)+" "+text)
}

func statusString(statusType v1.ActivityStatusType) string {
	text := statusType.String()
	switch statusType {
	case v1.ActivityStatusTypeFailed, v1.ActivityStatusTypeError:
		return termcolor.ColorError(text)
	case v1.ActivityStatusTypeSucceeded:
		return termcolor.ColorInfo(text)
	case v1.ActivityStatusTypeRunning:
		return termcolor.ColorStatus(text)
	}
	return text
}

func describePromotePullRequest(promote *v1.PromotePullRequestStep) string {
	description := ""
	if promote.PullRequestURL != "" {
		description += " PullRequest: " + termcolor.ColorInfo(promote.PullRequestURL)
	}
	if promote.MergeCommitSHA != "" {
		description += " Merge SHA: " + termcolor.ColorInfo(promote.MergeCommitSHA)
	}
	return description
}

func describePromoteUpdate(promote *v1.PromoteUpdateStep) string {
	description := ""
	for _, status := range promote.Statuses {
		url := status.URL
		state := status.Status

		if url != "" && state != "" {
			description += " Status: " + pullRequestStatusString(state) + " at: " + termcolor.ColorInfo(url)
		}
	}
	return description
}

func pullRequestStatusString(text string) string {
	title := strings.Title(text)
	switch text {
	case "success":
		return termcolor.ColorInfo(title)
	case "error", "failed":
		return termcolor.ColorError(title)
	default:
		return termcolor.ColorStatus(title)
	}
}

func timeToString(t *metav1.Time) string {
	if t == nil {
		return ""
	}
	now := &metav1.Time{
		Time: time.Now(),
	}
	return DurationString(t, now)
}

func (o *Options) matches(activity *v1.PipelineActivity) bool {
	answer := true
	filter := o.Filter
	if filter != "" {
		answer = strings.Contains(activity.Name, filter) || strings.Contains(activity.Spec.Pipeline, filter)
	}
	build := o.BuildNumber
	if answer && build != "" {
		answer = activity.Spec.Build == build
	}
	return answer
}

// DurationString returns the duration between start and end time as string
func DurationString(start, end *metav1.Time) string {
	if start == nil || end == nil {
		return ""
	}
	return end.Sub(start.Time).Round(time.Second).String()
}
