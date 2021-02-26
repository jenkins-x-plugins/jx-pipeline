package main

import (
	"github.com/jenkins-x/jx-pipeline/pkg/cmd/effective"
	"os"
)

// Entrypoint for the command
func main() {
	rootCmd, _ := effective.NewCmdPipelineEffective()
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}
