package grid

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/templates"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/jxclient"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/jxenv"
	"github.com/jenkins-x/jx-helpers/v3/pkg/options"
	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
	"os"
	"time"

	v1 "github.com/jenkins-x/jx-api/v4/pkg/apis/jenkins.io/v1"
	"github.com/jenkins-x/jx-api/v4/pkg/client/clientset/versioned"
	informers "github.com/jenkins-x/jx-api/v4/pkg/client/informers/externalversions"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"knative.dev/pkg/signals"
)

// Options containers the CLI options
type Options struct {
	options.BaseOptions

	Namespace  string
	Filter     string
	KubeClient kubernetes.Interface
	JXClient   versioned.Interface
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

	stop := signals.SetupSignalHandler()
	defer runtime.HandleCrash()

	m := newModel(o.Filter)
	p := tea.NewProgram(m)

	informer := informerFactory.Jenkins().V1().PipelineActivities().Informer()
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			e := obj.(*v1.PipelineActivity)
			m.onPipelineActivity(e)
			//log.Logger().Infof("added %s", e.Name)
		},
		UpdateFunc: func(old, new interface{}) {
			e := new.(*v1.PipelineActivity)
			m.onPipelineActivity(e)
			//log.Logger().Infof("updated %s", e.Name)
		},
		DeleteFunc: func(obj interface{}) {
			e := obj.(*v1.PipelineActivity)
			m.deletePipelineActivity(e.Name)
			//log.Logger().Infof("deleted %s", e.Name)
		},
	})
	informerFactory.Start(stop)
	if !cache.WaitForCacheSync(stop, informer.HasSynced) {
		msg := "timed out waiting for jx caches to sync"
		runtime.HandleError(fmt.Errorf(msg))
	}

	log.Logger().Infof("caches synchronised cache contains %d activities", m.count())

	log.Logger().Infof("starting tea")

	if err := p.Start(); err != nil {
		if err != nil {
			return errors.Wrapf(err, "failed to start viewer")
		}
	}

	log.Logger().Infof("tea completed")

	<-stop

	os.Exit(0)
	return nil
}
