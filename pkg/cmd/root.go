package cmd

import (
	"os"

	"github.com/jenkins-x/jx-helpers/pkg/cobras"
	"github.com/jenkins-x/jx-logging/pkg/log"
	"github.com/jenkins-x/jx-pipeline/pkg/cmd/version"
	"github.com/jenkins-x/jx-pipeline/pkg/rootcmd"
	"github.com/jenkins-x/jx/v2/pkg/cmd/clients"
	"github.com/jenkins-x/jx/v2/pkg/cmd/opts"
	"github.com/jenkins-x/jx/v2/pkg/cmd/get"
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

	g := get.NewCmdGetActivity(commonOpts)
	g.Short = "Display one or more pipeline activities"
	cmd.AddCommand(g)

	g = get.NewCmdGetPipeline(commonOpts)
	g.Short = "Display one or more pipelines"
	g.Use = "get"
	cmd.AddCommand(g)

	cmd.AddCommand(get.NewCmdGetBuildLogs(commonOpts))
	cmd.AddCommand(get.NewCmdGetBuildPods(commonOpts))
	cmd.AddCommand(get.NewCmdGetPreview(commonOpts))

	cmd.AddCommand(cobras.SplitCommand(version.NewCmdVersion()))
	return cmd
}
