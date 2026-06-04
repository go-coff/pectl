// Command pectl is the reference CLI for the go-coff/peln library and
// its appender + linker subpackages. It assembles, links and signs UEFI
// PE/COFF binaries — the cases that matter for Unified Kernel Image
// assembly.
//
// All command wiring lives in the cmd package; this file is just the
// process entry point.
package main

import (
	"os"

	"github.com/go-coff/pectl/cmd"
)

// osExit is indirected so tests can intercept the exit code.
var osExit = os.Exit

func main() {
	osExit(cmd.Execute())
}
