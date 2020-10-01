package wait

import (
	"time"

	"github.com/jenkins-x/go-scm/scm"
	"github.com/jenkins-x/jx-helpers/pkg/kube"
	"github.com/jenkins-x/jx-helpers/pkg/options"
	"github.com/jenkins-x/jx-helpers/pkg/termcolor"
	"github.com/jenkins-x/jx-pipeline/pkg/constants"
	"github.com/jenkins-x/jx-pipeline/pkg/triggers"
	"github.com/jenkins-x/lighthouse/pkg/config"
	"github.com/pkg/errors"
	"k8s.io/client-go/kubernetes"

	"github.com/jenkins-x/jx-helpers/pkg/cobras/helper"
	"github.com/spf13/cobra"

	"github.com/jenkins-x/jx-helpers/pkg/cobras/templates"
	"github.com/jenkins-x/jx-logging/pkg/log"
)

// Options contains the command line options
type Options struct {
	WaitDuration        time.Duration
	PollPeriod          time.Duration
	Owner               string
	Repository          string
	LighthouseConfigMap string
	Namespace           string
	KubeClient          kubernetes.Interface
}

var (
	info = termcolor.ColorInfo

	cmdLong = templates.LongDesc(`
		Waits for a pipeline to be imported and activated by the boot Job

`)

	cmdExample = templates.Examples(`
		# Waits for the pipeline to be setup for the given repository
		jx pipeline wait --owner myorg --repo myrepo
	`)
)

// NewCmdPipelineWait creates the command
func NewCmdPipelineWait() (*cobra.Command, *Options) {
	o := &Options{}

	cmd := &cobra.Command{
		Use:     "wait",
		Short:   "Waits for a pipeline to be imported and activated by the boot Job",
		Long:    cmdLong,
		Example: cmdExample,
		Aliases: []string{"build", "run"},
		Run: func(cmd *cobra.Command, args []string) {
			err := o.Run()
			helper.CheckErr(err)
		},
	}
	cmd.Flags().StringVarP(&o.Owner, "owner", "o", "", "The owner name to wait for")
	cmd.Flags().StringVarP(&o.Repository, "repo", "r", "", "The repository name o wait for")
	cmd.Flags().StringVarP(&o.LighthouseConfigMap, "configmap", "", constants.LighthouseConfigMapName, "The name of the Lighthouse ConfigMap to find the trigger configurations")
	cmd.Flags().StringVarP(&o.Namespace, "namespace", "n", "", "The namespace to look for the lighthouse configuration. Defaults to the current namespace")

	cmd.Flags().DurationVarP(&o.WaitDuration, "duration", "", time.Minute*20, "Maximum duration to wait for one or more matching triggers to be setup in Lighthouse. Useful for when a new repository is being imported via GitOps")
	cmd.Flags().DurationVarP(&o.PollPeriod, "poll-period", "", time.Second*2, "Poll period when waiting for one or more matching triggers to be setup in Lighthouse. Useful for when a new repository is being imported via GitOps")

	return cmd, o
}

// Validate verifies things are setup correctly
func (o *Options) Validate() error {
	var err error
	o.KubeClient, o.Namespace, err = kube.LazyCreateKubeClientAndNamespace(o.KubeClient, o.Namespace)
	if err != nil {
		return errors.Wrapf(err, "failed to create kube client")
	}

	if o.Owner == "" {
		return options.MissingOption("owner")
	}
	if o.Repository == "" {
		return options.MissingOption("repo")
	}
	return nil
}

// Run implements this command
func (o *Options) Run() error {
	err := o.Validate()
	if err != nil {
		return errors.Wrapf(err, "failed to validate options")
	}

	fullName := scm.Join(o.Owner, o.Repository)

	exists, err := o.waitForRepositoryToBeSetup(o.KubeClient, o.Namespace, fullName)
	if err != nil {
		return errors.Wrapf(err, "failed to get trigger names")
	}
	if !exists {
		return errors.Errorf("reppository %s is not yet setup in lighthouse", fullName)
	}

	log.Logger().Infof("the repository %s is now setup in lighthouse", info(fullName))
	return nil
}

func (o *Options) waitForRepositoryToBeSetup(kubeClient kubernetes.Interface, ns, fullName string) (bool, error) {
	end := time.Now().Add(o.WaitDuration)
	name := o.LighthouseConfigMap
	logWaiting := false

	for {
		cfg, err := triggers.LoadLighthouseConfig(kubeClient, ns, name, true)
		if err != nil {
			return false, errors.Wrapf(err, "failed to load lighthouse config")
		}
		flag := o.containsRepositoryTrigger(cfg, fullName)
		if flag {
			return flag, nil
		}

		if time.Now().After(end) {
			return false, errors.Errorf("failed to find trigger in the lighthouse configuration in ConfigMap %s in namespace %s for repository: %s within %s", name, ns, fullName, o.WaitDuration.String())
		}

		if !logWaiting {
			logWaiting = true
			log.Logger().Infof("waiting up to %s for a trigger to be added to the lighthouse configuration in ConfigMap %s in namespace %s for repository: %s", info(o.WaitDuration.String()), info(name), info(ns), info(fullName))
		}
		time.Sleep(o.PollPeriod)
	}
}

// containsRepositoryTrigger returns true if the trigger is setup for the repository
func (o *Options) containsRepositoryTrigger(cfg *config.Config, fullName string) bool {
	if cfg.Postsubmits[fullName] != nil {
		return true
	}
	if cfg.InRepoConfig.Enabled != nil {
		f := cfg.InRepoConfig.Enabled[fullName]
		if f != nil {
			return *f
		}
	}
	return false
}
