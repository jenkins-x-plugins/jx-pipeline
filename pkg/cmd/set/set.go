package set

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/jenkins-x-plugins/jx-pipeline/pkg/pipelines/processor"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/v3/pkg/options"
	"github.com/jenkins-x/jx-helpers/v3/pkg/termcolor"
	"github.com/pkg/errors"

	"github.com/spf13/cobra"

	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/templates"
)

// Options contains the command line options
type Options struct {
	options.BaseOptions

	Dir          string
	Filter       string
	TemplateEnvs []string

	templateEnvMap map[string]string
}

var (
	info = termcolor.ColorInfo

	cmdLong = templates.LongDesc(`
		Sets a property on the given Pipeline / PipelineRun / Task files.

`)

	cmdExample = templates.Examples(`
		# Modifies one or more Pipeline / PipelineRun / Tasks in the given folder
		jx pipeline set --dir tasks --template-env FOO=bar
	`)
)

// NewCmdPipelineSet creates the command
func NewCmdPipelineSet() (*cobra.Command, *Options) {
	o := &Options{}

	cmd := &cobra.Command{
		Use:     "set",
		Short:   "Sets a property on the given Pipeline / PipelineRun / Task files",
		Long:    cmdLong,
		Example: cmdExample,
		Run: func(cmd *cobra.Command, args []string) {
			err := o.Run()
			helper.CheckErr(err)
		},
	}
	cmd.Flags().StringVarP(&o.Dir, "dir", "d", ".", "Directory to look for YAML files")
	cmd.Flags().StringVarP(&o.Filter, "filter", "f", "", "Text filter to filter the YAML files to modify")
	cmd.Flags().StringArrayVarP(&o.TemplateEnvs, "template-env", "t", nil, "List of environment variables to set of the form 'NAME=value' on the step template")

	return cmd, o
}

// Run implements this command
func (o *Options) Validate() error {
	err := o.BaseOptions.Validate()
	if err != nil {
		return errors.Wrapf(err, "failed to validate base options")
	}

	if o.templateEnvMap == nil {
		o.templateEnvMap = map[string]string{}
	}
	for _, e := range o.TemplateEnvs {
		values := strings.SplitN(e, "=", 2)
		if len(values) != 2 {
			if err != nil {
				return options.InvalidOptionf("template-env", e, "should be of the form 'NAME=value'")
			}
		}
		o.templateEnvMap[values[0]] = values[1]
	}
	return nil
}

// Run implements this command
func (o *Options) Run() error {
	err := o.Validate()
	if err != nil {
		return errors.Wrapf(err, "failed to validate options")
	}

	err = filepath.Walk(o.Dir, func(path string, f os.FileInfo, err error) error {
		if f == nil || f.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return nil
		}
		if o.Filter != "" && !strings.Contains(path, o.Filter) {
			return nil
		}
		return o.modifyPipeline(path)
	})
	if err != nil {
		return errors.Wrapf(err, "failed to process files in dir %s", o.Dir)
	}
	return nil
}

func (o *Options) modifyPipeline(path string) error {
	p := processor.NewModifier(o.templateEnvMap)
	_, err := processor.ProcessFile(p, path)
	if err != nil {
		return errors.Wrapf(err, "failed to process file %s", path)
	}
	return nil
}
