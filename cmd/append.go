package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/go-coff/peln/appender"
	"github.com/spf13/cobra"
)

// namedFlag describes one of the well-known UKI sections that we expose as a
// dedicated shortcut flag, so callers don't have to remember the dotted
// section names.
type namedFlag struct {
	flag    string // CLI long flag, e.g. "linux"
	section string // PE section name, e.g. ".linux"
	help    string // one-line help shown by --help
}

var ukiNamed = []namedFlag{
	{"linux", ".linux", "kernel image (vmlinuz / vmlinuz.efi / bzImage)"},
	{"initrd", ".initrd", "initramfs (typically initramfs.cpio.gz)"},
	{"cmdline", ".cmdline", "kernel command line"},
	{"osrel", ".osrel", "os-release metadata"},
	{"uname", ".uname", "kernel uname -r string"},
}

// newAppendCmd builds the `pectl append` subcommand: append sections to
// a PE/COFF image. The companion to `pectl sign`; both pieces of the UKI
// assembly pipeline sit behind a subcommand so the CLI shape is uniform.
func newAppendCmd() *cobra.Command {
	var (
		output     string
		namedPaths = make([]string, len(ukiNamed)) // parallel to ukiNamed
		sections   []string                        // --section name=path values
		legacyAdds []string                        // hidden --add-section alias
	)

	cmd := &cobra.Command{
		Use:   "append [flags] INPUT",
		Short: "Append PE/COFF sections to a UEFI stub to assemble a UKI",
		Long: `append adds sections to a PE/COFF image. It is a minimal stand-in
for "objcopy --add-section", restricted to image-section appending — the case
that matters for UEFI Unified Kernel Image (UKI) assembly.

Named shortcuts (--linux, --initrd, --cmdline, --osrel, --uname) cover the
well-known UKI sections. For any other section name, use --section name=path
(repeatable).`,
		Example: `  # Build a UKI from a stub, a kernel, an initramfs and a cmdline.
  pectl append --linux=vmlinuz.efi --initrd=initramfs.cpio.gz \
               --cmdline=cmdline -o BOOTAA64.EFI stub.efi

  # Append a section with an arbitrary name.
  pectl append --section .splash=splash.bmp -o out.efi in.efi`,
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

			specs := make([]appender.Section, 0, len(ukiNamed)+len(sections)+len(legacyAdds))

			// Named shortcuts in canonical order. Empty paths skipped.
			for i, n := range ukiNamed {
				if namedPaths[i] == "" {
					continue
				}
				data, err := os.ReadFile(namedPaths[i])
				if err != nil {
					return err
				}
				specs = append(specs, appender.Section{
					Name:            n.section,
					Data:            data,
					Characteristics: appender.DefaultCharacteristics,
				})
			}

			// Generic name=path entries (new --section and legacy
			// --add-section). Order: --section first, then --add-section,
			// each in the order it was given on the command line.
			for _, raw := range append(append([]string{}, sections...), legacyAdds...) {
				name, path, err := splitNamePath(raw)
				if err != nil {
					return err
				}
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				specs = append(specs, appender.Section{
					Name:            name,
					Data:            data,
					Characteristics: appender.DefaultCharacteristics,
				})
			}

			if len(specs) == 0 {
				return usageErr{"no sections to append: use a named shortcut " +
					"(--linux, --initrd, ...) or --section name=path"}
			}

			stub, err := os.ReadFile(posArgs[0])
			if err != nil {
				return err
			}
			res, err := appender.Append(stub, specs)
			if err != nil {
				return err
			}
			return os.WriteFile(output, res, 0o644)
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "output PE image (required)")
	for i := range ukiNamed {
		n := ukiNamed[i]
		cmd.Flags().StringVar(&namedPaths[i], n.flag, "",
			fmt.Sprintf("path for section %s — %s", n.section, n.help))
	}
	cmd.Flags().StringArrayVarP(&sections, "section", "s", nil,
		"append section as name=path (repeatable; for arbitrary section names)")
	cmd.Flags().StringArrayVar(&legacyAdds, "add-section", nil,
		"alias for --section (kept for backwards compatibility)")
	_ = cmd.Flags().MarkHidden("add-section")

	return cmd
}

// splitNamePath parses a "name=path" CLI value. Both halves must be non-empty;
// the first '=' is the separator (so values may contain '=' freely).
func splitNamePath(v string) (name, path string, err error) {
	i := strings.IndexByte(v, '=')
	if i <= 0 || i == len(v)-1 {
		return "", "", usageErr{fmt.Sprintf("--section: expected name=path, got %q", v)}
	}
	return v[:i], v[i+1:], nil
}
