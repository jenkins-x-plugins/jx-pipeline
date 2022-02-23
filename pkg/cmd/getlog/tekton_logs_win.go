//go:build windows
// +build windows

package getlog

import (
	"os"
)

// This is required as the pager package has unix related functions
func (o *Options) setOutput() {
	if o.Out == nil {
		o.Out = os.Stdout
	}
}
