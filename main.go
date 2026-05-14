// Command pec is the reference CLI for the github.com/go-coff/pe library.
// It is a stand-in for `objcopy --add-section`, restricted to image-section
// appending — the case that matters for UEFI Unified Kernel Image assembly.
//
// Usage:
//
//	pec --add-section name=path [--add-section ...] -o out.efi in.efi
//
// Each --add-section flag takes "<sectname>=<file>"; the section is appended
// to the input PE with pe.DefaultCharacteristics (read-only initialised data).
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/go-coff/pe"
)

type addSpec struct{ Name, Path string }

type addList []addSpec

func (l *addList) String() string { return fmt.Sprintf("%v", *l) }
func (l *addList) Set(v string) error {
	i := strings.IndexByte(v, '=')
	if i <= 0 || i == len(v)-1 {
		return fmt.Errorf("expected name=path, got %q", v)
	}
	*l = append(*l, addSpec{Name: v[:i], Path: v[i+1:]})
	return nil
}

// osExit is indirected so tests can intercept the exit code.
var osExit = os.Exit

func main() {
	osExit(run(os.Args[1:], os.Stderr))
}

func run(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("pec", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var adds addList
	fs.Var(&adds, "add-section", "name=path (repeatable)")
	out := fs.String("o", "", "output file (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *out == "" || fs.NArg() != 1 || len(adds) == 0 {
		fmt.Fprintln(stderr,
			"usage: pec --add-section name=path [...] -o out.efi in.efi")
		return 2
	}

	stub, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	secs := make([]pe.Section, 0, len(adds))
	for _, a := range adds {
		data, err := os.ReadFile(a.Path)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		secs = append(secs, pe.Section{
			Name:            a.Name,
			Data:            data,
			Characteristics: pe.DefaultCharacteristics,
		})
	}
	res, err := pe.Append(stub, secs)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := os.WriteFile(*out, res, 0o644); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
