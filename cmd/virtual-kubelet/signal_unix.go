// +build !windows

package main

import (
	"os"
	"syscall"
)

func cancelSigs() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}
