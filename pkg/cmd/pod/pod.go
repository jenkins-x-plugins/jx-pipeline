package pod

import (
	"os"
	"strings"
	"time"

	"github.com/jenkins-x/jx-api/pkg/client/clientset/versioned"
	"github.com/jenkins-x/jx-helpers/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/pkg/kube"
	"github.com/jenkins-x/jx-helpers/pkg/kube/jxclient"
	"github.com/jenkins-x/jx-helpers/pkg/options"
	"github.com/jenkins-x/jx-helpers/pkg/table"
	"github.com/jenkins-x/jx-kube-client/pkg/kubeclient"
	"github.com/pkg/errors"
	tektonclient "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	"k8s.io/client-go/kubernetes"

	"github.com/jenkins-x/jx-helpers/pkg/cobras/templates"
	"github.com/jenkins-x/jx-logging/pkg/log"
	"github.com/jenkins-x/jx/v2/pkg/builds"
	"github.com/spf13/cobra"
)

// Options the command line options
type Options struct {
	options.BaseOptions

	KubeClient   kubernetes.Interface
	JXClient     versioned.Interface
	TektonClient tektonclient.Interface
	Format       string
	Namespace    string
	BuildFilter  builds.BuildPodInfoFilter
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
			err := o.Run()
			helper.CheckErr(err)
		},
	}
	cmd.Flags().StringVarP(&o.Namespace, "namespace", "n", "", "The namespace to look for the build pods. Defaults to the current namespace")
	cmd.Flags().BoolVarP(&o.BuildFilter.Pending, "pending", "p", false, "Filter builds which are currently pending or running")
	cmd.Flags().StringVarP(&o.BuildFilter.Filter, "filter", "f", "", "Filters the build name by the given text")
	cmd.Flags().StringVarP(&o.BuildFilter.Owner, "owner", "o", "", "Filters the owner (person/organisation) of the repository")
	cmd.Flags().StringVarP(&o.BuildFilter.Repository, "repo", "r", "", "Filters the build repository")
	cmd.Flags().StringVarP(&o.BuildFilter.Branch, "branch", "", "", "Filters the branch")
	cmd.Flags().StringVarP(&o.BuildFilter.Build, "build", "", "", "Filter a specific build number")
	cmd.Flags().StringVarP(&o.BuildFilter.Context, "context", "", "", "Filters the context of the build")
	cmd.Flags().StringVarP(&o.BuildFilter.GitURL, "giturl", "g", "", "The git URL to filter on. If you specify a link to a github repository or PR we can filter the query of build pods accordingly")

	o.BaseOptions.AddBaseFlags(cmd)
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
	return nil
}

// Run implements this command
func (o *Options) Run() error {
	err := o.Validate()
	if err != nil {
		return errors.Wrapf(err, "failed to validate options")
	}

	kubeClient := o.KubeClient
	ns := o.Namespace

	pods, err := builds.GetBuildPods(kubeClient, ns)
	if err != nil {
		log.Logger().Warnf("Failed to query pods %s", err)
		return err
	}

	out := os.Stdout
	t := table.CreateTable(out)
	t.AddRow("OWNER", "REPOSITORY", "BRANCH", "BUILD", "CONTEXT", "AGE", "STATUS", "POD", "GIT URL")

	var buildInfos []*builds.BuildPodInfo
	for _, pod := range pods {
		buildInfo := builds.CreateBuildPodInfo(pod)
		if o.BuildFilter.BuildMatches(buildInfo) {
			buildInfos = append(buildInfos, buildInfo)
		}
	}
	builds.SortBuildPodInfos(buildInfos)

	now := time.Now()
	for _, build := range buildInfos {
		duration := strings.TrimSuffix(now.Sub(build.CreatedTime).Round(time.Minute).String(), "0s")

		t.AddRow(build.Organisation, build.Repository, build.Branch, build.Build, build.Context, duration, build.Status(), build.PodName, build.GitURL)
	}
	t.Render()
	return nil
}
