// Package cmd implements the pectl command-line interface. pectl
// assembles, links and signs UEFI PE/COFF binaries:
//
//	pectl append    add .linux / .initrd / .cmdline / ... sections to a stub
//	pectl link      link COFF/PE relocatables into a PE32+ EFI application
//	pectl sign      Authenticode-sign a PE/COFF image for SecureBoot
//
// All three subcommands are pure Go — no objcopy, no lld-link, no sbsign,
// no host toolchain beyond the standard library and a handful of crypto
// helpers.
package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

// Execute is the canonical Cobra entry point. It parses os.Args[1:], writes
// diagnostics to os.Stderr, and returns the process exit code:
//
//	0  success
//	1  runtime error (read/write/append/link/sign failure)
//	2  usage error  (bad flag, missing argument, malformed --section value)
func Execute() int {
	return run(os.Args[1:], os.Stderr)
}

// usageErr signals a CLI misuse and maps to exit code 2. Shared by every
// subcommand — they return one of these from RunE or from their flag-error
// hook to opt into the usage exit code.
type usageErr struct{ msg string }

func (e usageErr) Error() string { return e.msg }

// newRootCmd builds the top-level command. It owns nothing of its own
// (no flags, no RunE); calling `pectl` bare prints help.
func newRootCmd(stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pectl",
		Short: "Build, link and sign UEFI PE/COFF binaries",
		Long: `pectl assembles, links and signs UEFI PE/COFF binaries.

  pectl append    add .linux / .initrd / .cmdline / ... sections to a stub
  pectl link      link COFF/PE relocatables into a PE32+ EFI application
  pectl sign      Authenticode-sign a PE/COFF image for SecureBoot

All three subcommands are pure Go — no objcopy, no lld-link, no sbsign
on the host.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.SetOut(stderr)
	cmd.SetErr(stderr)
	// pflag errors (unknown flag, missing value) and Args validator errors
	// should surface as usage errors with exit code 2. The hook is set at
	// root level; cobra walks up from the failing subcommand to find it.
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return usageErr{err.Error()}
	})
	cmd.AddCommand(newAppendCmd())
	cmd.AddCommand(newLinkCmd())
	cmd.AddCommand(newLinkPIECmd())
	cmd.AddCommand(newObjcopyCmd())
	cmd.AddCommand(newSignCmd())
	return cmd
}

func run(args []string, stderr io.Writer) int {
	cmd := newRootCmd(stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	if err == nil {
		return 0
	}
	var ue usageErr
	if errors.As(err, &ue) {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}
	fmt.Fprintln(stderr, err)
	return 1
}
