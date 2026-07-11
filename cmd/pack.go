// Copyright (c) 2026 The go-coff Authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that
// can be found in the LICENSE file.

package cmd

import (
	"compress/flate"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/go-coff/efipack"
)

// newPackCmd builds the `pectl pack` subcommand: take a single
// PE32+/EFI binary and emit a self-extracting PE32+/EFI envelope
// (the go-coff/efipack runtime-decompressor wrapper). Designed as
// the user-facing entry point for cloud-boot's M6.2 EFI compressor —
// the UPX-equivalent that does not exist anywhere else for this
// format.
//
// The compressor enum is exposed as -c <name> instead of a numeric
// id so the CLI is stable across efipack library bumps (Compressor
// ints can be reordered; names are wire-format-anchored).
func newPackCmd() *cobra.Command {
	var (
		output         string
		compressorName string
		level          int
	)

	cmd := &cobra.Command{
		Use:   "pack [flags] INPUT.efi",
		Short: "Compress a PE32+ EFI binary into a self-extracting PE32+ EFI envelope",
		Long: `pack reads a PE32+/EFI image, compresses its body with the chosen
codec, and emits a self-extracting PE32+/EFI envelope: a per-arch
runtime decompressor stub (.stub) + the compressed body (.payload).
At firmware load time the stub decompresses .payload into pool memory
and chain-boots it via gBS->LoadImage + gBS->StartImage.

Supported arches (auto-detected from the input's COFF Machine field):
amd64, arm64, riscv64, loong64. arm64 / riscv64 / loong64 produce
runnable envelopes (arm64 is OVMF-boot-verified); the amd64 runtime
stub currently faults on entry (X64 #UD) and is tracked for a follow-up
rebuild, so amd64 envelopes are structurally valid but not yet bootable.

Codecs:
  flate   stdlib compress/flate (default; zero stub-size cost). The
          embedded runtime stubs decode this tag, so -c flate is the
          only codec that produces a runnable packed EFI today.
  lzfse   github.com/go-compressions/lzfse — best ratio, host-side only.
  lz4     github.com/go-compressions/lz4 pure-Go block codec — fastest
          decompress, host-side only.

Both lzfse and lz4 compress + round-trip correctly on the host, but the
embedded runtime decompressor stubs decode FLAT exclusively, so a packed
binary produced with -c lzfse or -c lz4 will NOT boot under firmware
until a stub that decodes that algo tag ships. Use -c flate for a
runnable packed EFI.`,
		Example: `  # Default: flate at compress/flate.DefaultCompression.
  pectl pack -o BOOTAA64-packed.EFI BOOTAA64.EFI

  # Maximum compression, explicit codec name.
  pectl pack -c flate --level 9 -o out.efi in.efi`,
		Args: func(c *cobra.Command, a []string) error {
			if err := cobra.ExactArgs(1)(c, a); err != nil {
				return usageErr{err.Error()}
			}
			return nil
		},
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(c *cobra.Command, posArgs []string) error {
			if output == "" {
				return usageErr{"-o/--output is required"}
			}
			comp, err := parseCompressorName(compressorName)
			if err != nil {
				return usageErr{err.Error()}
			}
			// Reject obviously-out-of-range levels up front so users get
			// a usage error rather than the codec's internal complaint
			// at Encode time. flate accepts [-2, 9]; we use the same
			// window for the other codecs since their levels overlap.
			if level < -2 || level > 9 {
				return usageErr{fmt.Sprintf("-level: %d out of range (want -2..9)", level)}
			}

			in, err := os.Open(posArgs[0])
			if err != nil {
				return err
			}
			defer in.Close()

			out, err := os.Create(output)
			if err != nil {
				return err
			}
			defer out.Close()

			// Both lzfse and lz4 are wired host-side only: the embedded
			// per-arch runtime decompressor stubs (.stub) decode the
			// FLAT algo tag exclusively, so a packed binary produced
			// with -c lzfse or -c lz4 will not boot under firmware until
			// a stub that decodes that tag ships. Warn the user up front
			// so they don't ship an unbootable EFI by surprise — and so
			// the workaround (-c flate) is obvious.
			if comp == efipack.LZFSE || comp == efipack.LZ4 {
				fmt.Fprintf(c.ErrOrStderr(),
					"pectl pack: WARNING: -c %s produces a host-side-only packed binary; "+
						"the embedded runtime decompressor stub decodes FLAT only, so the "+
						"resulting EFI will not boot under firmware until a %s-aware stub "+
						"ships. Use -c flate for a runnable packed EFI.\n", comp, comp)
			}

			res, err := efipack.Pack(in, out, efipack.Options{
				Compressor: comp,
				Level:      level,
			})
			if err != nil {
				return err
			}

			// One-line summary so a caller can pipe / grep on it from a
			// build script. Reduction is computed against the input on
			// disk (not against the compressed body alone) so the user
			// sees the wall-clock saving the envelope produces.
			reduction := 0.0
			if res.OriginalSize > 0 {
				reduction = float64(res.OriginalSize-res.PackedSize) / float64(res.OriginalSize) * 100.0
			}
			fmt.Fprintf(c.OutOrStdout(),
				"pack: %s (%d bytes) -> %s (%d bytes, %.1f%% reduction, compressor=%s)\n",
				posArgs[0], res.OriginalSize, output, res.PackedSize, reduction, comp)
			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "output PE32+ envelope (required)")
	cmd.Flags().StringVarP(&compressorName, "compressor", "c", "flate", "compression algorithm: flate, lzfse, lz4")
	cmd.Flags().IntVar(&level, "level", flate.DefaultCompression,
		"compression level passed to the codec (default -1 = codec default)")

	return cmd
}

// parseCompressorName maps the CLI compressor names to efipack
// Compressor constants. The names are wire-format-anchored ("flate",
// "lzfse", "lz4"); aliases are intentionally not accepted to keep
// the codec identity that the .payload "algo" tag carries
// unambiguous.
func parseCompressorName(s string) (efipack.Compressor, error) {
	switch s {
	case "flate":
		return efipack.Flate, nil
	case "lzfse":
		return efipack.LZFSE, nil
	case "lz4":
		return efipack.LZ4, nil
	default:
		return 0, fmt.Errorf("-c/--compressor: %q (want flate, lzfse, lz4)", s)
	}
}
