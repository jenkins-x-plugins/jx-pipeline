package cmd

import (
	"os"

	"github.com/jenkins-x/jx-helpers/pkg/cobras"
	"github.com/jenkins-x/jx-logging/pkg/log"
	"github.com/jenkins-x/jx-pipeline/pkg/cmd/get"
	"github.com/jenkins-x/jx-pipeline/pkg/cmd/start"
	"github.com/jenkins-x/jx-pipeline/pkg/cmd/stop"
	"github.com/jenkins-x/jx-pipeline/pkg/cmd/version"
	"github.com/jenkins-x/jx-pipeline/pkg/rootcmd"
	"github.com/jenkins-x/jx/v2/pkg/cmd/clients"
	getold "github.com/jenkins-x/jx/v2/pkg/cmd/get"
	"github.com/jenkins-x/jx/v2/pkg/cmd/opts"
	"github.com/spf13/cobra"
)

// Main creates the new command
func Main() *cobra.Command {
	cmd := &cobra.Command{
		Use:   rootcmd.TopLevelCommand,
		Short: "commands for working with Jenkins X Pipelines",
		Run: func(cmd *cobra.Command, args []string) {
			err := cmd.Help()
			if err != nil {
				log.Logger().Errorf(err.Error())
			}
		},
	}
	f := clients.NewFactory()
	commonOpts := opts.NewCommonOptionsWithTerm(f, os.Stdin, os.Stdout, os.Stderr)
	commonOpts.AddBaseFlags(cmd)

	g := getold.NewCmdGetActivity(commonOpts)
	g.Short = "Display one or more pipeline activities"
	cmd.AddCommand(g)

	cmd.AddCommand(getold.NewCmdGetBuildLogs(commonOpts))
	cmd.AddCommand(getold.NewCmdGetBuildPods(commonOpts))

	cmd.AddCommand(cobras.SplitCommand(get.NewCmdPipelineGet()))
	cmd.AddCommand(cobras.SplitCommand(start.NewCmdPipelineStart(commonOpts)))
	cmd.AddCommand(cobras.SplitCommand(stop.NewCmdPipelineStop(commonOpts)))
	cmd.AddCommand(cobras.SplitCommand(version.NewCmdVersion()))
	return cmd
}
