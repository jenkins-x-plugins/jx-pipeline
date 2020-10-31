package lint

import (
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/jenkins-x/jx-api/v3/pkg/client/clientset/versioned"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/v3/pkg/files"
	"github.com/jenkins-x/jx-helpers/v3/pkg/input"
	"github.com/jenkins-x/jx-helpers/v3/pkg/options"
	"github.com/jenkins-x/jx-helpers/v3/pkg/yamls"
	"github.com/jenkins-x/lighthouse/pkg/config/job"
	"github.com/jenkins-x/lighthouse/pkg/triggerconfig"
	"github.com/pkg/errors"
	tektonclient "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	"k8s.io/client-go/kubernetes"

	"github.com/spf13/cobra"

	gojenkins "github.com/jenkins-x/golang-jenkins"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/templates"
)

// StopPipelineOptions contains the command line options
type Options struct {
	options.BaseOptions

	Dir          string
	Namespace    string
	Input        input.Interface
	KubeClient   kubernetes.Interface
	JXClient     versioned.Interface
	TektonClient tektonclient.Interface

	Jobs map[string]gojenkins.Job
}

var (
	cmdLong = templates.LongDesc(`
		Lints the lighthouse trigger and tekton pipelines
`)

	cmdExample = templates.Examples(`
		# Lints the lighthouse files and local pipeline files
		jx pipeline lint
	`)
)

// NewCmdPipelineLint creates the command
func NewCmdPipelineLint() (*cobra.Command, *Options) {
	o := &Options{}

	cmd := &cobra.Command{
		Use:     "lint",
		Short:   "Lints the lighthouse trigger and tekton pipelines",
		Long:    cmdLong,
		Example: cmdExample,
		Aliases: []string{"kill"},
		Run: func(cmd *cobra.Command, args []string) {
			err := o.Run()
			helper.CheckErr(err)
		},
	}
	cmd.Flags().StringVarP(&o.Dir, "dir", "d", ".", "The directory to look for the .lighthouse folder")

	return cmd, o
}

// Validate verifies things are setup correctly
func (o *Options) Validate() error {

	return nil
}

// Run implements this command
func (o *Options) Run() error {
	err := o.Validate()
	if err != nil {
		return errors.Wrapf(err, "failed to validate options")
	}

	dir := filepath.Join(o.Dir, ".lighthouse")
	fs, err := ioutil.ReadDir(dir)
	if err != nil {
		return errors.Wrapf(err, "failed to read dir %s", dir)
	}
	for _, f := range fs {
		name := f.Name()
		if !f.IsDir() || strings.HasPrefix(name, ".") {
			continue
		}

		triggerDir := filepath.Join(dir, name)
		triggersFile := filepath.Join(triggerDir, "triggers.yaml")
		exists, err := files.FileExists(triggersFile)
		if err != nil {
			return errors.Wrapf(err, "failed to check if file exists %s", triggersFile)
		}
		if !exists {
			continue
		}

		triggers := &triggerconfig.Config{}
		err = yamls.LoadFile(triggersFile, triggers)
		if err != nil {
			return errors.Wrapf(err, "failed to ")
		}

		_, err = loadConfigFile(triggers, triggerDir)
		if err != nil {
			return errors.Wrapf(err, "failed to load triggers file %s", triggersFile)
		}
	}
	return nil
}

func loadConfigFile(repoConfig *triggerconfig.Config, dir string) (*triggerconfig.Config, error) {
	for i := range repoConfig.Spec.Presubmits {
		r := &repoConfig.Spec.Presubmits[i]
		if r.SourcePath != "" {
			err := loadJobBaseFromSourcePath(filepath.Join(dir, r.SourcePath) + ".yaml")
			if err != nil {
				return nil, errors.Wrapf(err, "failed to load Source for Presubmit %d", i)
			}

		}
		if r.Agent == "" && r.PipelineRunSpec != nil {
			r.Agent = job.TektonPipelineAgent
		}
	}
	for i := range repoConfig.Spec.Postsubmits {
		r := &repoConfig.Spec.Postsubmits[i]
		if r.SourcePath != "" {
			err := loadJobBaseFromSourcePath(filepath.Join(dir, r.SourcePath) + ".yaml")
			if err != nil {
				return nil, errors.Wrapf(err, "failed to load Source for Presubmit %d", i)
			}
		}
		if r.Agent == "" && r.PipelineRunSpec != nil {
			r.Agent = job.TektonPipelineAgent
		}
	}
	return repoConfig, nil
}

func loadJobBaseFromSourcePath(path string) error {
	/* TODO
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return errors.Wrapf(err, "failed to load file %s", path)
	}
	if len(data) == 0 {
		return errors.Errorf("empty file file %s", path)
	}


	dir := filepath.Dir(path)

	message := fmt.Sprintf("file %s", path)

	getData := func(name string) ([]byte, error) {
		path := filepath.Join(dir, name)
		data, err := ioutil.ReadFile(path)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read file %s", path)
		}
		return data, nil
	}

	_, err = inrepo.LoadTektonResourceAsPipelineRun(data, dir, message, getData, nil)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal YAML file %s", path)
	}
	*/
	return nil
}
