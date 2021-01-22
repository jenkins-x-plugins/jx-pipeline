package lint

import (
	"context"
	"github.com/jenkins-x/jx-helpers/v3/pkg/scmhelpers"
	"github.com/jenkins-x/jx-pipeline/pkg/lighthouses"
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
	linter.Options
	ScmOptions scmhelpers.Options

	Namespace string
	OutFile   string
	Format    string
	Recursive bool
	Resolver  *inrepo.UsesResolver
}

var (
	info = termcolor.ColorInfo

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
		Run: func(cmd *cobra.Command, args []string) {
			err := o.Run()
			helper.CheckErr(err)
		},
	}
	o.ScmOptions.DiscoverFromGit = true
	cmd.Flags().StringVarP(&o.ScmOptions.Dir, "dir", "d", ".", "The directory to look for the .lighthouse folder")
	cmd.Flags().BoolVarP(&o.Recursive, "recursive", "r", false, "Recurisvely find all '.lighthouse' folders such as if linting a Pipeline Catalog")

	o.Options.AddFlags(cmd)

	return cmd, o
}

// Validate verifies settings
func (o *Options) Validate() error {
	err := o.Options.Validate()
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

	return o.LogResults()
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

		test := &linter.Test{
			File: triggersFile,
		}
		o.Tests = append(o.Tests, test)
		triggers := &triggerconfig.Config{}
		err = yamls.LoadFile(triggersFile, triggers)
		if err != nil {
			test.Error = err
			continue
		}

		o.loadConfigFile(triggers, triggerDir)
	}
	return nil
}

func (o *Options) loadConfigFile(repoConfig *triggerconfig.Config, dir string) *triggerconfig.Config {
	ctx := o.GetContext()
	for i := range repoConfig.Spec.Presubmits {
		r := &repoConfig.Spec.Presubmits[i]
		if r.SourcePath != "" {
			path := filepath.Join(dir, r.SourcePath)
			test := &linter.Test{
				File: path,
			}
			o.Tests = append(o.Tests, test)
			err := loadJobBaseFromSourcePath(ctx, o.Resolver, path)
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
			o.Tests = append(o.Tests, test)
			err := loadJobBaseFromSourcePath(ctx, o.Resolver, path)
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

func loadJobBaseFromSourcePath(ctx context.Context, resolver *inrepo.UsesResolver, path string) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return errors.Wrapf(err, "failed to load file %s", path)
	}
	if len(data) == 0 {
		return errors.Errorf("empty file file %s", path)
	}

	dir := filepath.Dir(path)
	resolver.Dir = dir
	pr, err := inrepo.LoadTektonResourceAsPipelineRun(resolver, data)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal YAML file %s", path)
	}

	fieldError := pr.Validate(ctx)
	if fieldError != nil {
		return errors.Wrapf(fieldError, "failed to validate YAML file %s", path)
	}
	return nil
}
