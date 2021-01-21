package effective

import (
	"github.com/jenkins-x/jx-helpers/v3/pkg/input"
	"github.com/jenkins-x/jx-helpers/v3/pkg/input/inputfactory"
	"github.com/jenkins-x/jx-helpers/v3/pkg/options"
	"github.com/jenkins-x/jx-helpers/v3/pkg/scmhelpers"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
	"github.com/jenkins-x/jx-pipeline/pkg/lighthouses"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"io/ioutil"
	"os"
	"path/filepath"
	"sigs.k8s.io/yaml"
	"strings"

	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/templates"
	"github.com/jenkins-x/jx-helpers/v3/pkg/files"
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

	Namespace    string
	OutFile      string
	TriggerName  string
	PipelineName string
	Recursive    bool
	Resolver     *inrepo.UsesResolver
	Triggers     []*Trigger
	Input        input.Interface
}

var (
	info = termcolor.ColorInfo

	cmdLong = templates.LongDesc(`
		Displays the effective tekton pipeline
`)

	cmdExample = templates.Examples(`
		# View the effective pipeline 
		jx pipeline effective
	`)
)

// Trigger the found trigger configs
type Trigger struct {
	Path      string
	Config    *triggerconfig.Config
	Names     []string
	Pipelines map[string]*tektonv1beta1.PipelineRun
}

// NewCmdPipelineEffective creates the command
func NewCmdPipelineEffective() (*cobra.Command, *Options) {
	o := &Options{}

	cmd := &cobra.Command{
		Use:     "effective",
		Short:   "Displays the effective tekton pipeline",
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
	cmd.Flags().StringVarP(&o.TriggerName, "trigger", "t", "", "The path to the trigger file. If not specified you will be prompted to choose one")
	cmd.Flags().StringVarP(&o.PipelineName, "pipeline", "p", "", "The pipeline kind and name. e.g. 'presubmit/pr' or 'postsubmit/release'. If not specified you will be prompted to choose one")
	cmd.Flags().StringVarP(&o.OutFile, "out", "o", "", "The output file to write the effective pipeline to. If not specified output to the terminal")
	cmd.Flags().BoolVarP(&o.Recursive, "recursive", "r", false, "Recurisvely find all '.lighthouse' folders such as if linting a Pipeline Catalog")

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
			Pipelines: map[string]*tektonv1beta1.PipelineRun{},
		}
		o.Triggers = append(o.Triggers, trigger)

		err = o.loadTriggerPipelines(trigger, triggerDir)
		if err != nil {
			return errors.Wrapf(err, "failed to load pipelines for trigger: %s", triggersFile)
		}
	}
	return nil
}

func (o *Options) loadTriggerPipelines(trigger *Trigger, dir string) error {
	repoConfig := trigger.Config
	for i := range repoConfig.Spec.Presubmits {
		r := &repoConfig.Spec.Presubmits[i]
		if r.SourcePath != "" {
			path := filepath.Join(dir, r.SourcePath)
			pr, err := lighthouses.LoadEffectivePipelineRun(o.Resolver, path)
			if err != nil {
				return errors.Wrapf(err, "failed to load %s", path)
			}
			name := "presubmit/" + r.Name
			trigger.Names = append(trigger.Names, name)
			trigger.Pipelines[name] = pr
		}
		if r.Agent == "" && r.PipelineRunSpec != nil {
			r.Agent = job.TektonPipelineAgent
		}
	}
	for i := range repoConfig.Spec.Postsubmits {
		r := &repoConfig.Spec.Postsubmits[i]
		if r.SourcePath != "" {
			path := filepath.Join(dir, r.SourcePath)
			pr, err := lighthouses.LoadEffectivePipelineRun(o.Resolver, path)
			if err != nil {
				return errors.Wrapf(err, "failed to load %s", path)
			}
			name := "postsubmit/" + r.Name
			trigger.Names = append(trigger.Names, name)
			trigger.Pipelines[name] = pr
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
	if pipeline == nil {
		return options.InvalidOptionf("pipeline", o.PipelineName, "available names %s", strings.Join(trigger.Names, ", "))
	}

	return o.displayPipeline(trigger, pipelineName, pipeline)
}

func (o *Options) displayPipeline(trigger *Trigger, name string, pipeline *tektonv1beta1.PipelineRun) error {
	if o.OutFile != "" {
		err := yamls.SaveFile(pipeline, o.OutFile)
		if err != nil {
			return errors.Wrapf(err, "failed to save file %s", o.OutFile)
		}
		log.Logger().Infof("saved file %s", info(o.OutFile))
		return nil
	}

	data, err := yaml.Marshal(pipeline)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal pipeline for %s", name)
	}

	log.Logger().Infof("trigger %s pipeline %s", info(trigger.Path), info(name))
	log.Logger().Infof(string(data))
	return nil
}
