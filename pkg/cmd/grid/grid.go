package grid

import (
	"context"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/pipelines"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/tektonlog"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/templates"
	"github.com/jenkins-x/jx-helpers/v3/pkg/input"
	"github.com/jenkins-x/jx-helpers/v3/pkg/input/inputfactory"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/jxclient"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/jxenv"
	"github.com/jenkins-x/jx-helpers/v3/pkg/options"
	"github.com/jenkins-x/jx-helpers/v3/pkg/termcolor"
	"github.com/jenkins-x/jx-kube-client/v3/pkg/kubeclient"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
	"github.com/spf13/cobra"
	tektonapis "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	tektonclient "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	v1 "github.com/jenkins-x/jx-api/v4/pkg/apis/jenkins.io/v1"
	"github.com/jenkins-x/jx-api/v4/pkg/client/clientset/versioned"
	informers "github.com/jenkins-x/jx-api/v4/pkg/client/informers/externalversions"

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
	Input          input.Interface
}

var (
	info = termcolor.ColorInfo

	cmdLong = templates.LongDesc(`
		Watches pipeline activity in a table

		You can use the up/down cursor keys to select a pipeline then hit enter on the selected pipeline to view its log. 
		When the pipeline is completed you can then go back to the pipeline grid and view other pipelines.
`)

	cmdExample = templates.Examples(`
		# Watches the current pipeline activities in a grid
		jx pipeline grid

		# Watches the current pipeline activities which have a name containing 'foo'
		jx pipeline grid -f foo
	`)
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
		Run: func(_ *cobra.Command, _ []string) {
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
		return fmt.Errorf("failed to create kube client: %w", err)
	}
	o.JXClient, err = jxclient.LazyCreateJXClient(o.JXClient)
	if err != nil {
		return fmt.Errorf("failed to create the jx client: %w", err)
	}
	if o.TektonClient == nil {
		f := kubeclient.NewFactory()
		cfg, err := f.CreateKubeConfig()
		if err != nil {
			return fmt.Errorf("failed to get kubernetes config: %w", err)
		}
		o.TektonClient, err = tektonclient.NewForConfig(cfg)
		if err != nil {
			return fmt.Errorf("error building tekton client: %w", err)
		}
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
		return fmt.Errorf("failed to validate options: %w", err)
	}

	ns, _, err := jxenv.GetDevNamespace(o.KubeClient, o.Namespace)
	if err != nil {
		return fmt.Errorf("failed to find dev namespace: %w", err)
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

	m := newModel(o.Filter, o.viewLogsFor)

	informer := informerFactory.Jenkins().V1().PipelineActivities().Informer()
	_, err = informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			e := obj.(*v1.PipelineActivity)
			if e != nil {
				m.onPipelineActivity(e)
			}
		},
		UpdateFunc: func(_, new interface{}) {
			e := new.(*v1.PipelineActivity)
			if e != nil {
				m.onPipelineActivity(e)
			}
		},
		DeleteFunc: func(obj interface{}) {
			e := obj.(*v1.PipelineActivity)
			if e != nil {
				m.deletePipelineActivity(e.Name)
			}
		},
	})
	if err != nil {
		return fmt.Errorf("failed to add handler for updated pipeline activities: %w", err)
	}
	informerFactory.Start(stop)
	if !cache.WaitForCacheSync(stop, informer.HasSynced) {
		msg := "timed out waiting for jx caches to sync"
		return fmt.Errorf("%s", msg)
	}

	p := tea.NewProgram(m)
	if err := p.Start(); err != nil {
		if err != nil {
			return fmt.Errorf("failed to start viewer: %w", err)
		}
	}
	return nil
}

func (o *Options) viewLogsFor(act *v1.PipelineActivity, paList []v1.PipelineActivity) error {
	if act == nil {
		return nil
	}
	log.Logger().Infof("\n\n")

	ns := o.Namespace
	if o.TektonLogger == nil {
		o.TektonLogger = &tektonlog.TektonLogger{
			KubeClient:     o.KubeClient,
			TektonClient:   o.TektonClient,
			JXClient:       o.JXClient,
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
		return fmt.Errorf("failed to list PipelineRuns in namespace %s: %w", ns, err)
	}
	if resources == nil {
		return fmt.Errorf("no PipelineRun resources found for namespace %s", ns)
	}

	var prList []*tektonapis.PipelineRun
	for i := range resources.Items {
		pr := &resources.Items[i]

		paName := pipelines.ToPipelineActivityName(pr, paList)
		if paName == act.Name {
			prList = append(prList, pr)
			break
		}
	}
	out := os.Stdout
	err = o.TektonLogger.GetLogsForActivity(ctx, out, act, act.Name, prList)
	if err != nil {
		return fmt.Errorf("failed to stream logs for pipeline %s: %w", act.Name, err)
	}

	fmt.Fprint(out, "\n\n")
	return nil
}
