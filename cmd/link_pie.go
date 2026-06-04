package cmd

import (
	"bytes"
	"fmt"
	"os"

	"github.com/go-coff/peln/linker"
	"github.com/spf13/cobra"
)

// newLinkPIECmd builds the `pectl link-pie` subcommand: take a single
// position-independent ELF executable (ET_DYN, e.g. produced by
// `go build -buildmode=pie` under TamaGo) and convert it to a PE32+ EFI
// application. Unlike `link`, there are no objects to merge and no symbols
// to resolve — the input is already linked; only its RELATIVE dynamic
// relocations are translated to PE base relocations.
func newLinkPIECmd() *cobra.Command {
	var (
		output       string
		subsystem    uint16
		imageBaseStr string
		sectionAlign uint32
		fileAlign    uint32
	)

	cmd := &cobra.Command{
		Use:   "link-pie [flags] PIE-ELF",
		Short: "Convert a position-independent ELF (ET_DYN) into a PE32+ EFI application",
		Long: `link-pie takes one already-linked position-independent ELF executable
(ET_DYN) whose dynamic relocations are all the architecture's RELATIVE
type — as produced by "go build -buildmode=pie" for a bare-metal GOOS such
as TamaGo — and emits a self-contained PE32+ EFI application.

It maps each PT_LOAD segment to a PE section, pre-applies every RELATIVE
relocation, and records an equivalent PE base relocation so UEFI firmware
can rebase the image at load time. Supported machines: amd64, arm64,
riscv64, loongarch64 (detected from the ELF header). The entry point comes
from the ELF's e_entry; subsystem defaults to 10 (EFI application).`,
		Example: `  # Convert a TamaGo loong64 PIE into a UEFI application.
  pectl link-pie -o BOOTLOONG64.EFI hello-pie.elf`,
		Args: func(c *cobra.Command, a []string) error {
			if err := cobra.ExactArgs(1)(c, a); err != nil {
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
			var imageBase uint64
			if imageBaseStr != "" {
				v, err := parseUint(imageBaseStr, 64)
				if err != nil {
					return usageErr{fmt.Sprintf("--image-base: %v", err)}
				}
				imageBase = v
			}

			data, err := os.ReadFile(posArgs[0])
			if err != nil {
				return err
			}
			out, err := linker.LinkPIE(bytes.NewReader(data), linker.PIEOptions{
				Subsystem:        subsystem,
				ImageBase:        imageBase,
				SectionAlignment: sectionAlign,
				FileAlignment:    fileAlign,
			})
			if err != nil {
				return err
			}
			return os.WriteFile(output, out, 0o644)
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "output PE image (required)")
	cmd.Flags().Uint16Var(&subsystem, "subsystem", 0,
		"PE Subsystem number (default 10 = EFI application)")
	cmd.Flags().StringVar(&imageBaseStr, "image-base", "",
		"preferred image base, decimal or 0x-hex (default: lowest segment vaddr floored to 64 KiB)")
	cmd.Flags().Uint32Var(&sectionAlign, "section-alignment", 0,
		"section alignment override (default 0x1000)")
	cmd.Flags().Uint32Var(&fileAlign, "file-alignment", 0,
		"file alignment override (default 0x200)")

	return cmd
}
