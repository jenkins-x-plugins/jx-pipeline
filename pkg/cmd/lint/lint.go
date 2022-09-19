package lint

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jenkins-x/jx-logging/v3/pkg/log"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/yaml"

	"github.com/jenkins-x-plugins/jx-pipeline/pkg/lighthouses"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/templates"
	"github.com/jenkins-x/jx-helpers/v3/pkg/files"
	"github.com/jenkins-x/jx-helpers/v3/pkg/linter"
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
	lighthouses.ResolverOptions

	Namespace string
	OutFile   string
	Format    string
	Recursive bool
	All       bool
	Resolver  *inrepo.UsesResolver
}

var (
	pipelineKinds = map[string]bool{
		"Pipeline":    true,
		"PipelineRun": true,
		"Task":        true,
		"TaskRun":     true,
	}

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
	o.ResolverOptions.AddFlags(cmd)

	cmd.Flags().BoolVarP(&o.Recursive, "recursive", "r", false, "Recurisvely find all '.lighthouse' folders such as if linting a Pipeline Catalog")
	cmd.Flags().BoolVarP(&o.All, "all", "a", false, "Rather than looking for .lighthouse and triggers.yaml files it looks for all YAML files which are tekton kinds")

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
		o.Resolver, err = o.ResolverOptions.CreateResolver()
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

	rootDir := o.Dir

	if o.All {
		err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info == nil || info.IsDir() || !strings.HasSuffix(info.Name(), ".yaml") {
				return nil
			}
			return o.ProcessFile(path)
		})
		if err != nil {
			return err
		}
	} else if o.Recursive {
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

func (o *Options) ProcessFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return errors.Wrapf(err, "failed to load file %s", path)
	}
	if len(data) == 0 {
		return errors.Errorf("empty file: %s", path)
	}

	u := &unstructured.Unstructured{}
	err = yaml.Unmarshal(data, u)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal file %s", path)
	}

	kind := u.GetKind()
	if !pipelineKinds[kind] {
		log.Logger().Debugf("ignoring file %s for unknown kind %s", path, kind)
		return nil
	}

	test := &linter.Test{
		File: path,
	}
	o.Tests = append(o.Tests, test)

	dir := filepath.Dir(path)
	o.Resolver.Dir = dir
	pr, err := inrepo.LoadTektonResourceAsPipelineRun(o.Resolver, data)
	if err != nil {
		test.Error = err
		return nil
	}
	ctx := o.GetContext()
	fieldError := ValidatePipelineRun(ctx, pr)
	if fieldError != nil {
		test.Error = fieldError
	}
	return nil
}

func ValidatePipelineRun(ctx context.Context, pr *v1beta1.PipelineRun) *apis.FieldError {
	err := pr.Validate(ctx)
	if err != nil {
		return err
	}

	// lets validate each TaskSpec has the right volumes etc
	ps := pr.Spec.PipelineSpec
	if ps == nil {
		return nil
	}
	for i := range ps.Tasks {
		pt := &ps.Tasks[i]
		if pt.TaskSpec == nil {
			continue
		}
		err = err.Also(ValidateTaskRunVolumesExist(&pt.TaskSpec.TaskSpec).ViaFieldIndex("tasks", i)).ViaField("spec", "pipelineSpec")
	}
	return err
}

func ValidateTaskRunVolumesExist(ts *v1beta1.TaskSpec) (errs *apis.FieldError) {
	volumeNames := map[string]bool{}
	for k := range ts.Volumes {
		volumeNames[ts.Volumes[k].Name] = true
	}

	for i := range ts.Steps {
		for j, v := range ts.Steps[i].VolumeMounts {
			if !volumeNames[v.Name] {
				errs = errs.Also(apis.ErrGeneric(fmt.Sprintf("Not found: %s", v.Name), "name").ViaFieldIndex("volumeMounts", j).ViaFieldIndex("steps", i).ViaField("taskSpec"))
			}
		}
	}
	return
}

func (o *Options) ProcessDir(dir string) error {
	fs, err := os.ReadDir(dir)
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
	data, err := os.ReadFile(path)
	if err != nil {
		return errors.Wrapf(err, "failed to load file %s", path)
	}
	if len(data) == 0 {
		return errors.Errorf("empty file: %s", path)
	}

	dir := filepath.Dir(path)
	resolver.Dir = dir
	pr, err := inrepo.LoadTektonResourceAsPipelineRun(resolver, data)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal YAML file %s", path)
	}

	fieldError := ValidatePipelineRun(ctx, pr)
	if fieldError != nil {
		return errors.Wrapf(fieldError, "failed to validate YAML file %s", path)
	}
	return nil
}
