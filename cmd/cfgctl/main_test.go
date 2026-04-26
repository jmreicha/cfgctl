package main

import (
	"os"
	"testing"
)

func TestMainCommand(_ *testing.T) {
	args := os.Args
	defer func() {
		os.Args = args
	}()

	os.Args = []string{"cfgctl", "--version"}
	main()
}
