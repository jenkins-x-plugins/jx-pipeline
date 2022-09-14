//go:build !windows
// +build !windows

package getlog

import (
	"os"

	"github.com/gerow/pager"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
)

func (o *Options) handleOutput(f func() error) error {
	if o.Out == nil {
		if !o.BatchMode {
			err := pager.Open()
			if err != nil {
				log.Logger().Debugf("Failed to use pager: %s", err)
			} else {
				defer pager.Close()
			}
		}
		o.Out = os.Stdout
	}
	return f()
}
