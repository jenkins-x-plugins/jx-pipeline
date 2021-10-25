package override

import (
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/jenkins-x-plugins/jx-pipeline/pkg/lighthouses"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/pipelines/processor"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/templates"
	"github.com/jenkins-x/jx-helpers/v3/pkg/files"
	"github.com/jenkins-x/jx-helpers/v3/pkg/input"
	"github.com/jenkins-x/jx-helpers/v3/pkg/input/inputfactory"
	"github.com/jenkins-x/jx-helpers/v3/pkg/options"
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
	lighthouses.ResolverOptions

	File             string
	Namespace        string
	CatalogSHA       string
	TriggerName      string
	PipelineName     string
	Step             string
	InlineProperties []string
	Resolver         *inrepo.UsesResolver
	Triggers         []*Trigger
	Input            input.Interface
}

var (
	cmdLong = templates.LongDesc(`
		Lets you pick a step to override locally in a pipeline
`)

	cmdExample = templates.Examples(`
		# Override locally a step in a pipeline
		jx pipeline override

		# Override the 'script' property from the property in the catalog
		# so that you can locally modfiy the script without locally maintaining all of the other properties such as image, env, resources etc
		jx pipeline override -P script 
	`)
)

// Trigger the found trigger configs
type Trigger struct {
	Path      string
	Config    *triggerconfig.Config
	Names     []string
	Pipelines map[string]string
}

// NewCmdPipelineOverride creates the command
func NewCmdPipelineOverride() (*cobra.Command, *Options) {
	o := &Options{}

	cmd := &cobra.Command{
		Use:     "override",
		Short:   "Lets you pick a step to override locally in a pipeline",
		Long:    cmdLong,
		Example: cmdExample,
		Aliases: []string{"edit", "inline"},
		Run: func(cmd *cobra.Command, args []string) {
			err := o.Run()
			helper.CheckErr(err)
		},
	}

	o.ResolverOptions.AddFlags(cmd)

	cmd.Flags().StringVarP(&o.File, "file", "f", "", "The pipeline file to render")
	cmd.Flags().StringVarP(&o.TriggerName, "trigger", "t", "", "The path to the trigger file. If not specified you will be prompted to choose one")
	cmd.Flags().StringVarP(&o.PipelineName, "pipeline", "p", "", "The pipeline kind and name. e.g. 'presubmit/pr' or 'postsubmit/release'. If not specified you will be prompted to choose one")
	cmd.Flags().StringVarP(&o.Step, "step", "s", "", "The name of the step to override")
	cmd.Flags().StringVarP(&o.CatalogSHA, "sha", "a", "HEAD", "The default catalog SHA to use when resolving catalog pipelines to reuse")
	cmd.Flags().StringArrayVarP(&o.InlineProperties, "properties", "P", nil, "The property names to override in the step. e.g. 'script' will just override the script tag")

	o.BaseOptions.AddBaseFlags(cmd)
	return cmd, o
}

// Validate verifies settings
func (o *Options) Validate() error {
	err := o.BaseOptions.Validate()
	if err != nil {
		return errors.Wrapf(err, "failed to validate base options")
	}
	if o.Input == nil {
		o.Input = inputfactory.NewInput(&o.BaseOptions)
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

	if o.File != "" {
		return o.overridePipeline(o.File)
	}

	rootDir := o.Dir

	dir := filepath.Join(rootDir, ".lighthouse")
	err = o.ProcessDir(dir)
	if err != nil {
		return err
	}
	return o.processTriggers()
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
			return errors.Wrapf(err, "failed to load %s", triggersFile)
		}
		trigger := &Trigger{
			Path:      triggersFile,
			Config:    triggers,
			Pipelines: map[string]string{},
		}
		o.Triggers = append(o.Triggers, trigger)

		err = o.loadTriggerPipelines(trigger, triggerDir)
		if err != nil {
			return errors.Wrapf(err, "failed to load pipelines for trigger: %s", triggersFile)
		}
	}
	return nil
}

func (o *Options) loadTriggerPipelines(trigger *Trigger, dir string) error { //nolint:unparam
	repoConfig := trigger.Config
	for i := range repoConfig.Spec.Presubmits {
		r := &repoConfig.Spec.Presubmits[i]
		if r.SourcePath != "" {
			path := filepath.Join(dir, r.SourcePath)
			name := "presubmit/" + r.Name
			trigger.Names = append(trigger.Names, name)
			trigger.Pipelines[name] = path
		}
		if r.Agent == "" && r.PipelineRunSpec != nil {
			r.Agent = job.TektonPipelineAgent
		}
	}
	for i := range repoConfig.Spec.Postsubmits {
		r := &repoConfig.Spec.Postsubmits[i]
		if r.SourcePath != "" {
			path := filepath.Join(dir, r.SourcePath)
			name := "postsubmit/" + r.Name
			trigger.Names = append(trigger.Names, name)
			trigger.Pipelines[name] = path
		}
		if r.Agent == "" && r.PipelineRunSpec != nil {
			r.Agent = job.TektonPipelineAgent
		}
	}
	return nil
}

func (o *Options) processTriggers() error {
	var names []string
	m := map[string]*Trigger{}
	for _, trigger := range o.Triggers {
		name := trigger.Path
		names = append(names, name)
		m[name] = trigger
	}

	var err error
	name := o.TriggerName
	if name == "" {
		name, err = o.Input.PickNameWithDefault(names, "pick the trigger config: ", "", "select the set of triggers to process")
		if err != nil {
			return errors.Wrapf(err, "failed to pick trigger file")
		}
		if name == "" {
			return errors.Errorf("no trigger file selected")
		}
	}
	trigger := m[name]
	if trigger == nil {
		return options.InvalidOptionf("trigger", o.TriggerName, "available names %s", strings.Join(names, ", "))
	}

	pipelineName := o.PipelineName
	if pipelineName == "" {
		pipelineName, err = o.Input.PickNameWithDefault(trigger.Names, "pick the pipeline: ", "", "select the pipeline to view")
		if err != nil {
			return errors.Wrapf(err, "failed to pick trigger file")
		}
		if pipelineName == "" {
			return errors.Errorf("no trigger file selected")
		}
	}
	pipeline := trigger.Pipelines[pipelineName]
	if pipeline == "" {
		return options.InvalidOptionf("pipeline", o.PipelineName, "available names %s", strings.Join(trigger.Names, ", "))
	}

	return o.overridePipeline(pipeline)
}

func (o *Options) overridePipeline(path string) error {
	p := processor.NewInliner(o.Input, o.Resolver, o.CatalogSHA, o.Step, o.InlineProperties)
	_, err := processor.ProcessFile(p, path)
	return err
}
