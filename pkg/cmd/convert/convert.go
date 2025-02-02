package convert

import (
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/templates"
	"github.com/jenkins-x/jx-helpers/v3/pkg/options"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
	"github.com/spf13/cobra"
)

// Options is the start of the data required to perform the operation.
// As new fields are added, add them here instead of
// referencing the cmd.Flags()
type Options struct {
	options.BaseOptions
}

var (
	convertCmdLong = templates.LongDesc(`
		Convert one or more pipelines.

`)

	convertCmdExample = templates.Examples(`
		# Convert a pipeline to use "image:uses:"
		jx pipeline convert uses
		# Convert a pipeline to use native Tekton
		jx pipeline convert remotetasks
	`)
)

// NewCmdPipelineConvert creates the command
func NewCmdPipelineConvert() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "convert",
		Short:   "commands for converting pipelines",
		Long:    convertCmdLong,
		Example: convertCmdExample,
		Run: func(cmd *cobra.Command, _ []string) {
			err := cmd.Help()
			if err != nil {
				log.Logger().Error(err.Error())
			}
		},
	}

	cmd.AddCommand(cobras.SplitCommand(NewCmdPipelineConvertUses()))
	cmd.AddCommand(cobras.SplitCommand(NewCmdPipelineConvertRemoteTasks()))
	return cmd
}
