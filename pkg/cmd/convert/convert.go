package convert

import (
	"github.com/jenkins-x/jx-helpers/v3/pkg/options"
	"github.com/jenkins-x/jx-helpers/v3/pkg/scmhelpers"
	"github.com/jenkins-x/jx-pipeline/pkg/lighthouses"
	"github.com/jenkins-x/jx-pipeline/pkg/pipelines/processor"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/templates"
	"github.com/jenkins-x/jx-helpers/v3/pkg/files"
	"github.com/jenkins-x/jx-helpers/v3/pkg/linter"
	"github.com/jenkins-x/jx-helpers/v3/pkg/termcolor"
	"github.com/jenkins-x/jx-helpers/v3/pkg/yamls"
	"github.com/jenkins-x/lighthouse-client/pkg/config/job"
	"github.com/jenkins-x/lighthouse-client/pkg/triggerconfig"
	"github.com/jenkins-x/lighthouse-client/pkg/triggerconfig/inrepo"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// Options contains the command line options
type Options struct {
	options.BaseOptions
	ScmOptions scmhelpers.Options

	Namespace   string
	TasksFolder string
	Format      string
	Recursive   bool
	Resolver    *inrepo.UsesResolver
	Processor   processor.Interface
}

var (
	info = termcolor.ColorInfo

	cmdLong = templates.LongDesc(`
		Converts the pipelines to use the 'image: uses:sourceURI' include mechanism

	So that pipeline catalogs copy smaller, simpler and easier to upgrade pipelines
`)

	cmdExample = templates.Examples(`
		# Recurisvely convert all the pipelines in the .lighthouse/*/*.yaml folders
		jx pipeline convert -r -d packs
	`)
)

// NewCmdPipelineConvert creates the command
func NewCmdPipelineConvert() (*cobra.Command, *Options) {
	o := &Options{}

	cmd := &cobra.Command{
		Use:     "convert",
		Short:   "Converts the pipelines to use the 'image: uses:sourceURI' include mechanism",
		Long:    cmdLong,
		Example: cmdExample,
		Aliases: []string{"kill"},
		Run: func(cmd *cobra.Command, args []string) {
			err := o.Run()
			helper.CheckErr(err)
		},
	}
	o.ScmOptions.DiscoverFromGit = true
	cmd.Flags().StringVarP(&o.ScmOptions.Dir, "dir", "d", ".", "The directory to look for the .lighthouse folder")
	cmd.Flags().StringVarP(&o.TasksFolder, "tasks-dir", "", "tasks", "The directory name to store the original tasks before we convert to uses: notation")
	cmd.Flags().BoolVarP(&o.Recursive, "recursive", "r", false, "Recurisvely find all '.lighthouse' folders such as if linting a Pipeline Catalog")

	return cmd, o
}

// Validate verifies settings
func (o *Options) Validate() error {
	err := o.BaseOptions.Validate()
	if err != nil {
		return errors.Wrapf(err, "failed to validate base options")
	}
	if o.Resolver == nil {
		o.Resolver, err = lighthouses.CreateResolver(&o.ScmOptions)
		if err != nil {
			return errors.Wrapf(err, "failed to create a UsesResolver")
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

	rootDir := o.ScmOptions.Dir
	if o.Processor == nil {
		o.Processor = processor.NewUsesMigrator(rootDir, o.TasksFolder, o.ScmOptions.Owner, o.ScmOptions.Repository)
	}
	if o.Recursive {
		err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info == nil || !info.IsDir() || info.Name() != ".lighthouse" {
				return nil
			}
			return o.ProcessDir(path)
		})
		if err != nil {
			return err
		}
	} else {
		dir := filepath.Join(rootDir, ".lighthouse")
		err := o.ProcessDir(dir)
		if err != nil {
			return err
		}
	}
	return nil
}

func (o *Options) ProcessDir(dir string) error {
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
			return errors.Wrapf(err, "failed to load lighthouse triggers: %s", triggersFile)
		}

		o.processTriggerFile(triggers, triggerDir)
	}
	return nil
}

func (o *Options) processTriggerFile(repoConfig *triggerconfig.Config, dir string) *triggerconfig.Config {
	for i := range repoConfig.Spec.Presubmits {
		r := &repoConfig.Spec.Presubmits[i]
		if r.SourcePath != "" {
			path := filepath.Join(dir, r.SourcePath)
			test := &linter.Test{
				File: path,
			}
			err := processor.ProcessFile(o.Processor, path)
			if err != nil {
				test.Error = err
			}
		}
		if r.Agent == "" && r.PipelineRunSpec != nil {
			r.Agent = job.TektonPipelineAgent
		}
	}
	for i := range repoConfig.Spec.Postsubmits {
		r := &repoConfig.Spec.Postsubmits[i]
		if r.SourcePath != "" {
			path := filepath.Join(dir, r.SourcePath)
			test := &linter.Test{
				File: path,
			}
			err := processor.ProcessFile(o.Processor, path)
			if err != nil {
				test.Error = err
			}
		}
		if r.Agent == "" && r.PipelineRunSpec != nil {
			r.Agent = job.TektonPipelineAgent
		}
	}
	return repoConfig
}
