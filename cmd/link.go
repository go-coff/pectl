package cmd

import (
	"bytes"
	"fmt"
	"os"
	"strconv"

	"github.com/go-coff/peln/linker"
	"github.com/spf13/cobra"
)

// machineFromName maps a friendly CLI name to the COFF IMAGE_FILE_MACHINE_*
// constant. Returning 0 means "auto-detect from the first input object".
func machineFromName(s string) (uint16, error) {
	switch s {
	case "":
		return 0, nil
	case "amd64", "x86_64", "x64":
		return 0x8664, nil
	case "arm64", "aarch64":
		return 0xaa64, nil
	case "riscv64":
		return 0x5064, nil
	case "loongarch64", "loong64":
		return 0x6264, nil
	default:
		return 0, fmt.Errorf("unknown machine %q (want amd64, arm64, riscv64, loongarch64)", s)
	}
}

// parseUint accepts decimal ("16384") and the C-style hex notation
// ("0x4000") for flag values, matching what lld-link does for /base.
func parseUint(s string, bits int) (uint64, error) {
	base := 10
	v := s
	if len(v) > 2 && (v[:2] == "0x" || v[:2] == "0X") {
		base = 16
		v = v[2:]
	}
	return strconv.ParseUint(v, base, bits)
}

// newLinkCmd builds the `pectl link` subcommand: take one or more COFF/PE
// relocatable .o files and produce a PE32+ EFI application. It is the
// pure-Go replacement for `lld-link /subsystem:efi_application`, motivated
// by riscv64 (which LLD's COFF driver does not support).
func newLinkCmd() *cobra.Command {
	var (
		output          string
		machineName     string
		entry           string
		subsystem       uint16
		imageBaseStr    string
		allowUnresolved bool
		sectionAlign    uint32
		fileAlign       uint32
		headerReserve   uint32
	)

	cmd := &cobra.Command{
		Use:   "link [flags] OBJECT...",
		Short: "Link COFF/PE relocatables into a PE32+ EFI application",
		Long: `link takes one or more COFF/PE relocatable .o files (as produced by
TinyGo or clang with a *-pc-windows-gnu LLVM triple) and produces a
self-contained PE32+ EFI application — the same job lld-link does with
/subsystem:efi_application, only in pure Go.

The machine is auto-detected from the first input unless --machine is
given. Supported machines: amd64, arm64, riscv64, loongarch64. The entry symbol
defaults to _start; subsystem defaults to 10 (EFI application).`,
		Example: `  # Link a TinyGo arm64 stub.
  pectl link -o BOOTAA64.EFI main-arm64.o thunk-arm64.o

  # Same, but allow unresolved externals (zero-filled) — handy when the
  # input objects reference libc / Win32 helpers the EFI runtime ignores.
  pectl link --allow-unresolved -o BOOTX64.EFI main-amd64.o thunk-amd64.o`,
		Args: func(c *cobra.Command, a []string) error {
			if err := cobra.MinimumNArgs(1)(c, a); err != nil {
				return usageErr{err.Error()}
			}
			return nil
		},
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(_ *cobra.Command, posArgs []string) error {
			if output == "" {
				return usageErr{"-o/--output is required"}
			}
			machine, err := machineFromName(machineName)
			if err != nil {
				return usageErr{err.Error()}
			}
			var imageBase uint64
			if imageBaseStr != "" {
				v, err := parseUint(imageBaseStr, 64)
				if err != nil {
					return usageErr{fmt.Sprintf("--image-base: %v", err)}
				}
				imageBase = v
			}

			objs := make([]*linker.Object, 0, len(posArgs))
			for _, p := range posArgs {
				data, err := os.ReadFile(p)
				if err != nil {
					return err
				}
				o, err := linker.ReadObject(bytes.NewReader(data), p)
				if err != nil {
					return err
				}
				objs = append(objs, o)
			}

			out, err := linker.Link(objs, linker.LinkOptions{
				Machine:          machine,
				Entry:            entry,
				Subsystem:        subsystem,
				ImageBase:        imageBase,
				AllowUnresolved:  allowUnresolved,
				SectionAlignment: sectionAlign,
				FileAlignment:    fileAlign,
				HeaderReserve:    headerReserve,
			})
			if err != nil {
				return err
			}
			return os.WriteFile(output, out, 0o644)
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "output PE image (required)")
	cmd.Flags().StringVar(&machineName, "machine", "",
		"target machine: amd64, arm64, riscv64, loongarch64 (default: auto-detect from first object)")
	cmd.Flags().StringVar(&entry, "entry", "", `entry symbol name (default "_start")`)
	cmd.Flags().Uint16Var(&subsystem, "subsystem", 0,
		"PE Subsystem number (default 10 = EFI application)")
	cmd.Flags().StringVar(&imageBaseStr, "image-base", "",
		"preferred image base, decimal or 0x-hex (default 0x10000)")
	cmd.Flags().BoolVar(&allowUnresolved, "allow-unresolved", false,
		"zero-fill unresolved external symbols instead of failing")
	cmd.Flags().Uint32Var(&sectionAlign, "section-alignment", 0,
		"section alignment override (default 0x1000)")
	cmd.Flags().Uint32Var(&fileAlign, "file-alignment", 0,
		"file alignment override (default 0x200)")
	cmd.Flags().Uint32Var(&headerReserve, "header-reserve", 0,
		"bytes reserved between section table and first section (default file-alignment)")

	return cmd
}
