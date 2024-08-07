package wait

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jenkins-x-plugins/jx-pipeline/pkg/constants"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/triggers"
	"github.com/jenkins-x/go-scm/scm"
	jxc "github.com/jenkins-x/jx-api/v4/pkg/client/clientset/versioned"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/jxclient"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/naming"
	"github.com/jenkins-x/jx-helpers/v3/pkg/options"
	"github.com/jenkins-x/jx-helpers/v3/pkg/termcolor"
	"github.com/jenkins-x/lighthouse-client/pkg/config"

	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/templates"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
)

// Options contains the command line options
type Options struct {
	options.BaseOptions

	WaitDuration        time.Duration
	PollPeriod          time.Duration
	Owner               string
	Repository          string
	LighthouseConfigMap string
	Namespace           string
	KubeClient          kubernetes.Interface
	JXClient            jxc.Interface
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
		Run: func(_ *cobra.Command, _ []string) {
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
		return fmt.Errorf("failed to create jx client: %w", err)
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
		return fmt.Errorf("failed to validate options: %w", err)
	}

	fullName := scm.Join(o.Owner, o.Repository)

	ctx := o.GetContext()
	exists, err := o.waitForRepositoryToBeSetup(ctx, o.KubeClient, o.Namespace, o.Owner, o.Repository)
	if err != nil {
		return fmt.Errorf("failed to wait for repository to be setup in lighthouse: %w", err)
	}
	if !exists {
		return fmt.Errorf("repository %s is not yet setup in lighthouse", fullName)
	}

	err = o.waitForWebHookToBeSetup(ctx, o.JXClient, o.Namespace, o.Owner, o.Repository)
	if err != nil {
		return fmt.Errorf("failed to wait for repository to have its webhook enabled: %w", err)
	}

	log.Logger().Infof("the repository %s is now setup in lighthouse and has its webhook enabled", info(fullName))
	return nil
}

func (o *Options) waitForRepositoryToBeSetup(ctx context.Context, kubeClient kubernetes.Interface, ns, owner, repo string) (bool, error) {
	end := time.Now().Add(o.WaitDuration)
	name := o.LighthouseConfigMap
	logWaiting := false
	fullName := scm.Join(o.Owner, o.Repository)

	for {
		cfg, err := triggers.LoadLighthouseConfig(ctx, kubeClient, ns, name, true)
		if err != nil {
			return false, fmt.Errorf("failed to load lighthouse config: %w", err)
		}
		flag := o.containsRepositoryTrigger(cfg, owner, repo)
		if flag {
			return flag, nil
		}

		if time.Now().After(end) {
			log.Logger().Info("")
			log.Logger().Warn("It looks like the boot job failed to setup this project.")
			log.Logger().Infof("You can view the log via: %s", info("jx admin log"))
			return false, fmt.Errorf("failed to find trigger in the lighthouse configuration in ConfigMap %s in namespace %s for repository: %s within %s", name, ns, fullName, o.WaitDuration.String())
		}

		if !logWaiting {
			logWaiting = true
			log.Logger().Info("")
			log.Logger().Infof("waiting up to %s for a trigger to be added to the lighthouse configuration in ConfigMap %s in namespace %s for repository: %s", info(o.WaitDuration.String()), info(name), info(ns), info(fullName))
			log.Logger().Infof("you can watch the boot job to update the configuration via: %s", info("jx admin log"))
			log.Logger().Info("for more information on how this works see: https://jenkins-x.io/v3/about/how-it-works/#importing--creating-quickstarts")
			log.Logger().Info("")
		}
		time.Sleep(o.PollPeriod)
	}
}

func (o *Options) waitForWebHookToBeSetup(ctx context.Context, jxClient jxc.Interface, ns, owner, repository string) error {
	end := time.Now().Add(o.WaitDuration)
	name := naming.ToValidName(o.Owner + "-" + o.Repository)
	logWaiting := false

	fullName := scm.Join(owner, repository)
	lastValue := ""
	found := false
	lastFailMessage := ""
	for {
		sr, err := jxClient.JenkinsV1().SourceRepositories(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return fmt.Errorf("failed to find SourceRepository %s in namespace %s: %w", name, ns, err)
			}
		} else {
			if !found {
				found = true
				log.Logger().Infof("found SourceRepository %s for %s", info(sr.Name), info(sr.Spec.URL))
			}

			if sr.Annotations == nil {
				sr.Annotations = map[string]string{}
			}
			value := sr.Annotations["webhook.jenkins-x.io"]
			if value != "" {
				if value != lastValue {
					lastValue = value
					log.Logger().Infof("webhook status annotation is: %s", info(value))

					if value == "true" {
						return nil
					}

					if strings.HasPrefix(strings.ToLower(value), "err") {
						failure := sr.Annotations["webhook.jenkins-x.io/error"]
						if failure != "" && failure != lastFailMessage {
							lastFailMessage = failure
							log.Logger().Warnf("when creating webhook: %s", lastFailMessage)
						}
					}
				}
			}
		}

		if time.Now().After(end) {
			log.Logger().Info("")
			log.Logger().Warn("It looks like the boot job failed to setup the webhooks. It could be related to the git token permissions.")
			log.Logger().Infof("You can view the log via: %s", info("jx admin log"))
			log.Logger().Info("")

			return fmt.Errorf("failed to find sourceRepository %s in namespace %s for repository: %s within %s", name, ns, fullName, o.WaitDuration.String())
		}

		if !logWaiting {
			logWaiting = true
			log.Logger().Infof("waiting up to %s the webhook to be registered for the SourceRepository %s in namespace %s for repository: %s", info(o.WaitDuration.String()), info(name), info(ns), info(fullName))
		}
		time.Sleep(o.PollPeriod)
	}
}

// containsRepositoryTrigger returns true if the trigger is setup for the repository
func (o *Options) containsRepositoryTrigger(cfg *config.Config, owner, repo string) bool {
	fullName := scm.Join(owner, repo)
	if cfg.Postsubmits[fullName] != nil {
		return true
	}

	if cfg.InRepoConfig.Enabled != nil {
		f := cfg.InRepoConfig.Enabled[fullName]
		if f != nil {
			return *f
		}
		f = cfg.InRepoConfig.Enabled[scm.Join(owner, strings.Replace(repo, "-", "_", -1))]
		if f != nil {
			log.Logger().Info("found trigger after converting dash to underscore in formatted repository name")
			return *f
		}
	}
	return false
}
