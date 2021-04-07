package grid

import (
	"context"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/pipelines"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/tektonlog"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/templates"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/jxclient"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/jxenv"
	"github.com/jenkins-x/jx-helpers/v3/pkg/options"
	"github.com/jenkins-x/jx-kube-client/v3/pkg/kubeclient"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
	"github.com/spf13/cobra"
	tektonapis "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	tektonclient "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"os"
	"time"

	v1 "github.com/jenkins-x/jx-api/v4/pkg/apis/jenkins.io/v1"
	"github.com/jenkins-x/jx-api/v4/pkg/client/clientset/versioned"
	informers "github.com/jenkins-x/jx-api/v4/pkg/client/informers/externalversions"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
)

// Options containers the CLI options
type Options struct {
	options.BaseOptions

	Namespace      string
	Filter         string
	FailIfPodFails bool
	KubeClient     kubernetes.Interface
	JXClient       versioned.Interface
	TektonClient   tektonclient.Interface
	TektonLogger   *tektonlog.TektonLogger
}

var (
	cmdLong = templates.LongDesc(`
		Watches pipeline activity in a table
`)

	cmdExample = templates.Examples(`
		# Watches the current pipeline activities in a grid
		jx pipeline grid

		# Watches the current pipeline activities which have a name containing 'foo'
		jx pipeline grid -f foo
	`)

	disable = false
)

// NewCmdPipelineGrid creates the new command
func NewCmdPipelineGrid() (*cobra.Command, *Options) {
	o := &Options{}

	cmd := &cobra.Command{
		Use:     "grid",
		Short:   "Watches pipeline activity in a table",
		Aliases: []string{"table", "tbl"},
		Long:    cmdLong,
		Example: cmdExample,
		Run: func(cmd *cobra.Command, args []string) {
			err := o.Run()
			helper.CheckErr(err)
		},
	}
	cmd.Flags().StringVarP(&o.Filter, "filter", "f", "", "Text to filter the pipeline names")
	cmd.Flags().BoolVarP(&o.FailIfPodFails, "fail-with-pod", "", false, "Return an error if the pod fails")

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

	ns, _, err := jxenv.GetDevNamespace(o.KubeClient, o.Namespace)
	if err != nil {
		return errors.Wrapf(err, "failed to find dev namespace")
	}
	jxClient := o.JXClient

	informerFactory := informers.NewSharedInformerFactoryWithOptions(
		jxClient,
		time.Minute*10,
		informers.WithNamespace(ns),
	)
	stop := make(chan struct{})

	defer close(stop)
	defer runtime.HandleCrash()

	m := newModel(o.Filter)

	informer := informerFactory.Jenkins().V1().PipelineActivities().Informer()
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			e := obj.(*v1.PipelineActivity)
			m.onPipelineActivity(e)
		},
		UpdateFunc: func(old, new interface{}) {
			e := new.(*v1.PipelineActivity)
			m.onPipelineActivity(e)
		},
		DeleteFunc: func(obj interface{}) {
			e := obj.(*v1.PipelineActivity)
			m.deletePipelineActivity(e.Name)
		},
	})
	informerFactory.Start(stop)
	if !cache.WaitForCacheSync(stop, informer.HasSynced) {
		msg := "timed out waiting for jx caches to sync"
		runtime.HandleError(fmt.Errorf(msg))
	}

	p := tea.NewProgram(m)
	if err := p.Start(); err != nil {
		if err != nil {
			return errors.Wrapf(err, "failed to start viewer")
		}
	}
	act := m.activityTable.selected()
	if act == nil {
		return nil
	}
	log.Logger().Infof("\n\n")

	if o.TektonLogger == nil {
		o.TektonLogger = &tektonlog.TektonLogger{
			KubeClient:     o.KubeClient,
			TektonClient:   o.TektonClient,
			JXClient:       jxClient,
			Namespace:      ns,
			FailIfPodFails: o.FailIfPodFails,
		}
	}
	ctx := context.TODO()

	resources, err := o.TektonClient.TektonV1beta1().PipelineRuns(ns).List(ctx, metav1.ListOptions{})
	if err != nil && apierrors.IsNotFound(err) {
		err = nil
	}
	if err != nil {
		return errors.Wrapf(err, "failed to list PipelineRuns in namespace %s", ns)
	}
	if resources == nil {
		return errors.Errorf("no PipelineRun resources found for namespace %s", ns)
	}

	paList := m.activityTable.activityList()
	var prList []*tektonapis.PipelineRun
	for i := range resources.Items {
		pr := &resources.Items[i]

		paName := pipelines.ToPipelineActivityName(pr, paList)
		if paName == act.Name {
			prList = append(prList, pr)
			break
		}
	}
	return o.TektonLogger.GetLogsForActivity(ctx, os.Stdout, act, act.Name, prList)
}
