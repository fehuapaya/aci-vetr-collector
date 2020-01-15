package main

import (
	"fmt"

	"github.com/alexflint/go-arg"
)

// Args are command line parameters.
type Args struct {
	APIC     string `arg:"-a" help:"APIC hostname or IP address"`
	Username string `arg:"-u" help:"APIC username"`
	Password string `arg:"-p" help:"APIC password"`
	Output   string `arg:"-o" help:"Output file"`
	ICurl    bool   `help:"Write requests to icurl script"`
}

// Description is the CLI description string.
func (Args) Description() string {
	return "ACI vetR collector"
}

// Version is the CLI version string.
func (Args) Version() string {
	return "version " + version
}

func newArgs() (Args, error) {
	args := Args{Output: resultZip}
	arg.MustParse(&args)
	if args.ICurl && args.APIC == "" {
		return args, fmt.Errorf("APIC host or IP is required for icurl script output")
	}
	return args, nil
}
