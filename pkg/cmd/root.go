package cmd

import (
	"os"

	"github.com/jenkins-x/jx-helpers/pkg/cobras"
	"github.com/jenkins-x/jx-logging/pkg/log"
	"github.com/jenkins-x/jx-pipeline/pkg/cmd/activities"
	"github.com/jenkins-x/jx-pipeline/pkg/cmd/get"
	"github.com/jenkins-x/jx-pipeline/pkg/cmd/getlog"
	"github.com/jenkins-x/jx-pipeline/pkg/cmd/pod"
	"github.com/jenkins-x/jx-pipeline/pkg/cmd/start"
	"github.com/jenkins-x/jx-pipeline/pkg/cmd/stop"
	"github.com/jenkins-x/jx-pipeline/pkg/cmd/version"
	"github.com/jenkins-x/jx-pipeline/pkg/cmd/wait"
	"github.com/jenkins-x/jx-pipeline/pkg/rootcmd"
	"github.com/jenkins-x/jx/v2/pkg/cmd/clients"
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

	cmd.AddCommand(cobras.SplitCommand(activities.NewCmdActivities()))
	cmd.AddCommand(cobras.SplitCommand(get.NewCmdPipelineGet()))
	cmd.AddCommand(cobras.SplitCommand(getlog.NewCmdGetBuildLogs()))
	cmd.AddCommand(cobras.SplitCommand(pod.NewCmdGetBuildPods()))
	cmd.AddCommand(cobras.SplitCommand(start.NewCmdPipelineStart(commonOpts)))
	cmd.AddCommand(cobras.SplitCommand(stop.NewCmdPipelineStop(commonOpts)))
	cmd.AddCommand(cobras.SplitCommand(wait.NewCmdPipelineWait()))
	cmd.AddCommand(cobras.SplitCommand(version.NewCmdVersion()))
	return cmd
}
