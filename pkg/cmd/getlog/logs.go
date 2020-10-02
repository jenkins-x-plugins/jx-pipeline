package getlog

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/jenkins-x/jx-api/pkg/client/clientset/versioned"
	"github.com/jenkins-x/jx-helpers/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/pkg/input"
	"github.com/jenkins-x/jx-helpers/pkg/input/inputfactory"
	"github.com/jenkins-x/jx-helpers/pkg/kube"
	"github.com/jenkins-x/jx-helpers/pkg/kube/jxclient"
	"github.com/jenkins-x/jx-helpers/pkg/options"
	"github.com/jenkins-x/jx-kube-client/pkg/kubeclient"
	"github.com/jenkins-x/jx-pipeline/pkg/logs"
	"github.com/jenkins-x/jx/v2/pkg/gits"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"

	"github.com/jenkins-x/jx-helpers/pkg/cobras/templates"
	"github.com/jenkins-x/jx-logging/pkg/log"
	"github.com/jenkins-x/jx/v2/pkg/util"
	tektonclient "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
)

// Options the command line options
type Options struct {
	options.BaseOptions

	Args                    []string
	Format                  string
	Namespace               string
	Tail                    bool
	Wait                    bool
	CurrentFolder           bool
	FailIfPodFails          bool
	WaitForPipelineDuration time.Duration
	BuildFilter             logs.BuildPodInfoFilter
	KubeClient              kubernetes.Interface
	JXClient                versioned.Interface
	TektonClient            tektonclient.Interface
	TektonLogger            *logs.TektonLogger
	Input                   input.Interface
	Out                     io.Writer
}

// CLILogWriter is an implementation of logs.LogWriter that will show logs in the standard output
type CLILogWriter struct {
}

var (
	cmdLong = templates.LongDesc(`
		Display a build log

`)

	cmdExample = templates.Examples(`
		# Display a build log - with the user choosing which repo + build to view
		jx pipeline log

		# Pick a build to view the log based on the repo cheese
		jx pipeline log --repo cheese

		# Pick a pending Tekton build to view the log based 
		jx pipeline log -p

		# Pick a pending Tekton build to view the log based on the repo cheese
		jx pipeline log --repo cheese -p

		# Pick a Tekton build for the 1234 Pull Request on the repo cheese
		jx pipeline log --repo cheese --branch PR-1234

		# View the build logs for a specific tekton build pod
		jx pipeline log --pod my-pod-name
	`)
)

// NewCmdGetBuildLogs creates the command
func NewCmdGetBuildLogs() (*cobra.Command, *Options) {
	o := &Options{}

	cmd := &cobra.Command{
		Use:     "log [flags]",
		Short:   "Display a build log",
		Long:    cmdLong,
		Example: cmdExample,
		Aliases: []string{"logs"},
		Run: func(cmd *cobra.Command, args []string) {
			o.Args = args
			err := o.Run()
			helper.CheckErr(err)
		},
	}
	cmd.Flags().BoolVarP(&o.Tail, "tail", "t", true, "Tails the build log to the current terminal")
	cmd.Flags().BoolVarP(&o.Wait, "wait", "w", false, "Waits for the build to start before failing")
	cmd.Flags().BoolVarP(&o.FailIfPodFails, "fail-with-pod", "", false, "Return an error if the pod fails")
	cmd.Flags().DurationVarP(&o.WaitForPipelineDuration, "wait-duration", "d", time.Minute*5, "Timeout period waiting for the given pipeline to be created")
	cmd.Flags().BoolVarP(&o.CurrentFolder, "current", "c", false, "Display logs using current folder as repo name, and parent folder as owner")

	o.BaseOptions.AddBaseFlags(cmd)
	o.BuildFilter.AddFlags(cmd)
	return cmd, o
}

// Validate verifies things are setup correctly
func (o *Options) Validate() error {
	if o.Out == nil {
		o.Out = os.Stdout
	}
	err := o.BuildFilter.Validate()
	if err != nil {
		return err
	}

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

	return o.getProwBuildLog(o.KubeClient, o.TektonClient, o.JXClient, o.Namespace)
}

// getProwBuildLog prompts the user, if needed, to choose a pipeline, and then prints out that pipeline's logs.
func (o *Options) getProwBuildLog(kubeClient kubernetes.Interface, tektonClient tektonclient.Interface, jxClient versioned.Interface, ns string) error {
	if o.CurrentFolder {
		currentDirectory, err := os.Getwd()
		if err != nil {
			return err
		}

		gitRepository, err := gits.NewGitCLI().Info(currentDirectory)
		if err != nil {
			return err
		}

		o.BuildFilter.Repository = gitRepository.Name
		o.BuildFilter.Owner = gitRepository.Organisation
	}

	var err error

	if o.TektonLogger == nil {
		o.TektonLogger = &logs.TektonLogger{
			KubeClient:     kubeClient,
			TektonClient:   tektonClient,
			JXClient:       jxClient,
			Namespace:      ns,
			FailIfPodFails: o.FailIfPodFails,
		}
	}
	var waitableCondition bool
	f := func() error {
		waitableCondition, err = o.getTektonLogs()
		return err
	}

	err = f()
	if err != nil {
		if o.Wait && waitableCondition {
			log.Logger().Info("The selected pipeline didn't start, let's wait a bit")
			err = util.Retry(o.WaitForPipelineDuration, f)
			if err != nil {
				return err
			}
		}
		return err
	}
	return nil
}

func (o *Options) getTektonLogs() (bool, error) {
	var defaultName string

	names, paMap, prMap, err := o.TektonLogger.GetTektonPipelinesWithActivePipelineActivity(&o.BuildFilter)
	if err != nil {
		return true, err
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

	if o.BatchMode {
		if len(filteredNames) > 1 {
			return false, errors.New("more than one pipeline returned in batch mode, use better filters and try again")
		}
		if len(filteredNames) == 1 {
			defaultName = filteredNames[0]
		}
	}

	name, err := o.Input.PickNameWithDefault(filteredNames, "Which build do you want to view the logs of?: ", defaultName, "")
	if err != nil {
		return len(filteredNames) == 0, err
	}

	pa, exists := paMap[name]
	prList := prMap[name]

	if !exists {
		return true, errors.New("there are no build logs for the supplied filters")
	}

	if pa.Spec.BuildLogsURL != "" {
		for line := range o.TektonLogger.StreamPipelinePersistentLogs(pa.Spec.BuildLogsURL, nil) {
			fmt.Fprintln(o.Out, line.Line)
		}
		return false, o.TektonLogger.Err()
	}

	log.Logger().Infof("Build logs for %s", util.ColorInfo(name))
	name = strings.TrimSuffix(name, " ")

	for line := range o.TektonLogger.GetRunningBuildLogs(pa, prList, name, false) {
		fmt.Fprintln(o.Out, line.Line)
	}
	return false, o.TektonLogger.Err()
}
