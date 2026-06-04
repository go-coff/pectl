package cmd

import (
	"bytes"
	"debug/elf"
	"fmt"
	"os"

	"github.com/go-coff/peln/fwimg"
	"github.com/spf13/cobra"
)

// uimageArchFromELF maps an ELF machine to the U-Boot uImage arch code,
// used as the default when --uimage-arch is not given.
func uimageArchFromELF(m elf.Machine) byte {
	switch m {
	case elf.EM_ARM:
		return fwimg.ArchARM
	case elf.EM_AARCH64:
		return fwimg.ArchARM64
	case elf.EM_X86_64:
		return fwimg.ArchX86_64
	case elf.EM_RISCV:
		return fwimg.ArchRISCV
	case elf.EM_LOONGARCH:
		return fwimg.ArchLoongArch
	default:
		return 0
	}
}

func uimageArchFromName(s string) (byte, error) {
	switch s {
	case "":
		return 0, nil // caller derives from the ELF
	case "arm":
		return fwimg.ArchARM, nil
	case "arm64", "aarch64":
		return fwimg.ArchARM64, nil
	case "x86_64", "amd64":
		return fwimg.ArchX86_64, nil
	case "riscv", "riscv64":
		return fwimg.ArchRISCV, nil
	case "loongarch", "loong64":
		return fwimg.ArchLoongArch, nil
	default:
		return 0, fmt.Errorf("unknown uimage arch %q", s)
	}
}

// newObjcopyCmd builds `pectl objcopy`: convert an ELF executable into a
// flat binary, Motorola S-record, Intel HEX, or legacy U-Boot uImage — the
// pure-Go stand-in for `objcopy -O ...` / `srec_cat` / `mkimage`, for
// non-UEFI bare-metal targets.
func newObjcopyCmd() *cobra.Command {
	var (
		output     string
		format     string
		useVaddr   bool
		padStr     string
		loadStr    string
		entryStr   string
		name       string
		uimageArch string
		uimageOS   uint8
		uimageType uint8
	)

	cmd := &cobra.Command{
		Use:   "objcopy [flags] INPUT.elf",
		Short: "Convert an ELF into a flat binary / SREC / Intel HEX / U-Boot uImage",
		Long: `objcopy converts an ELF executable into a non-UEFI bare-metal image — a
flat binary (like objcopy -O binary), a Motorola S-record, an Intel HEX
file, or a legacy U-Boot uImage (like mkimage) — in pure Go.

Use it for targets loaded at a fixed address rather than via UEFI: e.g. a
Raspberry Pi kernel.img, a QEMU -kernel image, a ROM/flash image, or a
bootm payload. (For UEFI use ` + "`pectl link`/`link-pie`" + ` instead.)`,
		Example: `  # Flat binary for a fixed-load bare-metal image.
  pectl objcopy -O binary -o kernel.img kernel.elf

  # Intel HEX for a flash programmer.
  pectl objcopy -O ihex -o fw.hex fw.elf

  # Legacy U-Boot kernel image.
  pectl objcopy -O uimage --load 0x80000 --entry 0x80000 --name linux -o uImage kernel.elf`,
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
			arch, err := uimageArchFromName(uimageArch)
			if err != nil {
				return usageErr{err.Error()}
			}
			data, err := os.ReadFile(posArgs[0])
			if err != nil {
				return err
			}
			ef, err := elf.NewFile(bytes.NewReader(data))
			if err != nil {
				return err
			}
			flat, base, err := fwimg.Flatten(bytes.NewReader(data), fwimg.FlattenOptions{
				UseVaddr: useVaddr,
				Pad:      parseByteOr(padStr),
			})
			if err != nil {
				return err
			}

			// Address overrides: default load = image base, entry = e_entry.
			load := uint32(base)
			if loadStr != "" {
				v, err := parseUint(loadStr, 32)
				if err != nil {
					return usageErr{fmt.Sprintf("--load: %v", err)}
				}
				load = uint32(v)
			}
			entry := uint32(ef.Entry)
			if entryStr != "" {
				v, err := parseUint(entryStr, 32)
				if err != nil {
					return usageErr{fmt.Sprintf("--entry: %v", err)}
				}
				entry = uint32(v)
			}

			var out []byte
			switch format {
			case "binary", "":
				out = flat
			case "srec":
				out = fwimg.SREC(load, flat, name, entry, 0)
			case "ihex":
				out = fwimg.IHEX(load, flat, 0)
			case "uimage":
				if arch == 0 {
					arch = uimageArchFromELF(ef.Machine)
				}
				out = fwimg.UImage(flat, fwimg.UImageOptions{
					Load: load, Entry: entry, Name: name,
					OS: uimageOS, Arch: arch, Type: uimageType,
				})
			default:
				return usageErr{fmt.Sprintf("unknown --output-target %q (want binary, srec, ihex, uimage)", format)}
			}
			return os.WriteFile(output, out, 0o644)
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "output file (required)")
	cmd.Flags().StringVarP(&format, "output-target", "O", "binary",
		"output format: binary, srec, ihex, uimage")
	cmd.Flags().BoolVar(&useVaddr, "use-vaddr", false,
		"place segments by virtual address instead of load address")
	cmd.Flags().StringVar(&padStr, "pad", "",
		"gap fill byte for -O binary, decimal or 0x-hex (default 0x00)")
	cmd.Flags().StringVar(&loadStr, "load", "",
		"load address for srec/ihex/uimage, decimal or 0x-hex (default: image base)")
	cmd.Flags().StringVar(&entryStr, "entry", "",
		"entry address for srec/uimage, decimal or 0x-hex (default: ELF e_entry)")
	cmd.Flags().StringVar(&name, "name", "", "image name (srec S0 / uimage header)")
	cmd.Flags().StringVar(&uimageArch, "uimage-arch", "",
		"uimage arch: arm, arm64, x86_64, riscv, loongarch (default: from ELF)")
	cmd.Flags().Uint8Var(&uimageOS, "uimage-os", 0, "uimage OS code (default 5 = Linux)")
	cmd.Flags().Uint8Var(&uimageType, "uimage-type", 0, "uimage type code (default 2 = kernel)")

	return cmd
}

// parseByteOr parses a decimal/0x-hex byte, returning 0 on empty/invalid
// (the pad byte is cosmetic; an invalid value falls back to zero fill).
func parseByteOr(s string) byte {
	if s == "" {
		return 0
	}
	v, err := parseUint(s, 8)
	if err != nil {
		return 0
	}
	return byte(v)
}
