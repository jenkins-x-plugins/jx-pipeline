package cmd

import (
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/cmd/activities"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/cmd/breakpoint"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/cmd/convert"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/cmd/effective"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/cmd/env"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/cmd/fmt"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/cmd/get"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/cmd/getlog"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/cmd/grid"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/cmd/importcmd"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/cmd/lint"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/cmd/override"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/cmd/pod"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/cmd/set"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/cmd/start"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/cmd/stop"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/cmd/version"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/cmd/wait"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/rootcmd"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
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

	cmd.AddCommand(cobras.SplitCommand(activities.NewCmdActivities()))
	cmd.AddCommand(cobras.SplitCommand(breakpoint.NewCmdPipelineBreakpoint()))
	cmd.AddCommand(convert.NewCmdPipelineConvert())
	cmd.AddCommand(cobras.SplitCommand(effective.NewCmdPipelineEffective()))
	cmd.AddCommand(cobras.SplitCommand(env.NewCmdPipelineEnv()))
	cmd.AddCommand(cobras.SplitCommand(get.NewCmdPipelineGet()))
	cmd.AddCommand(cobras.SplitCommand(getlog.NewCmdGetBuildLogs()))
	cmd.AddCommand(cobras.SplitCommand(grid.NewCmdPipelineGrid()))
	cmd.AddCommand(cobras.SplitCommand(fmt.NewCmdPipelineFormat()))
	cmd.AddCommand(cobras.SplitCommand(importcmd.NewCmdPipelineImport()))
	cmd.AddCommand(cobras.SplitCommand(lint.NewCmdPipelineLint()))
	cmd.AddCommand(cobras.SplitCommand(override.NewCmdPipelineOverride()))
	cmd.AddCommand(cobras.SplitCommand(pod.NewCmdGetBuildPods()))
	cmd.AddCommand(cobras.SplitCommand(set.NewCmdPipelineSet()))
	cmd.AddCommand(cobras.SplitCommand(start.NewCmdPipelineStart()))
	cmd.AddCommand(cobras.SplitCommand(stop.NewCmdPipelineStop()))
	cmd.AddCommand(cobras.SplitCommand(wait.NewCmdPipelineWait()))
	cmd.AddCommand(cobras.SplitCommand(version.NewCmdVersion()))
	return cmd
}
