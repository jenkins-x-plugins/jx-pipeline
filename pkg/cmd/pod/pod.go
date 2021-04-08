package pod

import (
	"os"
	"strings"
	"time"

	"github.com/jenkins-x-plugins/jx-pipeline/pkg/tektonlog"
	"github.com/jenkins-x/jx-api/v4/pkg/client/clientset/versioned"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/jxclient"
	"github.com/jenkins-x/jx-helpers/v3/pkg/options"
	"github.com/jenkins-x/jx-helpers/v3/pkg/table"
	"github.com/jenkins-x/jx-kube-client/v3/pkg/kubeclient"
	"github.com/pkg/errors"
	tektonclient "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	"k8s.io/client-go/kubernetes"

	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/templates"
	"github.com/spf13/cobra"
)

// Options the command line options
type Options struct {
	options.BaseOptions

	Args         []string
	Format       string
	Namespace    string
	BuildFilter  tektonlog.BuildPodInfoFilter
	KubeClient   kubernetes.Interface
	JXClient     versioned.Interface
	TektonClient tektonclient.Interface
	TektonLogger *tektonlog.TektonLogger
}

var (
	cmdLong = templates.LongDesc(`
		Display the Tekton build pods

`)

	cmdExample = templates.Examples(`
		# List all the Tekton build pods
		jx pipeline pods

		# List all the pending Tekton build pods 
		jx pipeline pods -p

		# List all the Tekton build pods for a given repository
		jx pipeline pods --repo cheese

		# List all the pending Tekton build pods for a given repository
		jx pipeline pods --repo cheese -p

		# List all the Tekton build pods for a given Pull Request
		jx pipeline pods --repo cheese --branch PR-1234
	`)
)

// NewCmdGetBuildPods creates the command
func NewCmdGetBuildPods() (*cobra.Command, *Options) {
	o := &Options{}

	cmd := &cobra.Command{
		Use:     "pods [flags]",
		Short:   "Displays the build pods and their details",
		Long:    cmdLong,
		Example: cmdExample,
		Aliases: []string{"pod"},
		Run: func(cmd *cobra.Command, args []string) {
			o.Args = args
			err := o.Run()
			helper.CheckErr(err)
		},
	}
	cmd.Flags().StringVarP(&o.Namespace, "namespace", "n", "", "The namespace to look for the build pods. Defaults to the current namespace")

	o.BaseOptions.AddBaseFlags(cmd)
	o.BuildFilter.AddFlags(cmd)
	return cmd, o
}

// Validate verifies things are setup correctly
func (o *Options) Validate() error {
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

	if o.TektonLogger == nil {
		o.TektonLogger = &tektonlog.TektonLogger{
			KubeClient:   o.KubeClient,
			TektonClient: o.TektonClient,
			JXClient:     o.JXClient,
			Namespace:    o.Namespace,
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

	names, paMap, _, err := o.TektonLogger.GetTektonPipelinesWithActivePipelineActivity(ctx, &o.BuildFilter)
	if err != nil {
		return err
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

	out := os.Stdout
	t := table.CreateTable(out)
	t.AddRow("OWNER", "REPOSITORY", "BRANCH", "BUILD", "CONTEXT", "AGE", "STATUS", "POD", "GIT URL")

	now := time.Now()
	for _, name := range filteredNames {
		pa := paMap[name]
		if pa == nil {
			continue
		}
		build := &pa.Spec
		duration := strings.TrimSuffix(now.Sub(pa.CreationTimestamp.Time).Round(time.Minute).String(), "0s")

		podName := pa.Labels["podName"]
		t.AddRow(build.GitOwner, build.GitRepository, build.GitBranch, build.Build, build.Context, duration, string(build.Status), podName, build.GitURL)
	}
	t.Render()
	return nil
}
