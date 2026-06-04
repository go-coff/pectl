package cmd

import (
	"crypto"
	"crypto/x509"
	"fmt"
	"os"

	"github.com/foxboron/go-uefi/authenticode"
	"github.com/foxboron/go-uefi/efi/util"
	"github.com/spf13/cobra"
)

// signPE is indirected so tests can force the (otherwise hard-to-reach)
// signing-error branch.
var signPE = func(b *authenticode.PECOFFBinary, key crypto.Signer, cert *x509.Certificate) error {
	_, err := b.Sign(key, cert)
	return err
}

// newSignCmd builds the `pectl sign` subcommand: Authenticode-sign a
// PE/COFF image so that a SecureBoot-enabled firmware whose db trusts the
// signing certificate will load and run it.
//
// The signing path is intentionally a separate subcommand from `pectl
// append` because signing MUST be the last step of UKI assembly — any
// post-signing edit invalidates the Authenticode digest. The expected
// pipeline is:
//
//	pectl append --linux=… --initrd=… --cmdline=… -o uki.efi stub.efi  # assemble
//	pectl sign --key=key.pem --cert=cert.pem -o signed.efi uki.efi
func newSignCmd() *cobra.Command {
	var (
		keyPath  string
		certPath string
		output   string
	)

	cmd := &cobra.Command{
		Use:   "sign [flags] INPUT",
		Short: "Authenticode-sign a PE/COFF image for SecureBoot",
		Long: `sign attaches a PKCS#7 / Authenticode signature to a PE/COFF image so
SecureBoot firmware whose db trusts the signing certificate will load and
run it. The implementation is pure Go (no sbsign / sbsigntools dependency).

The signature is appended to the PE's attribute certificate table; the
existing sections are left untouched. Run this LAST in the UKI pipeline
— any post-signing edit (including pectl append) invalidates the digest.`,
		Example: `  # Generate a self-signed cert once (openssl / step / your CA flow),
  # then sign a UKI assembled with the root command.
  pectl sign --key=db.key --cert=db.crt -o BOOTAA64-signed.EFI BOOTAA64-real.EFI`,
		Args: func(c *cobra.Command, a []string) error {
			if err := cobra.ExactArgs(1)(c, a); err != nil {
				return usageErr{err.Error()}
			}
			return nil
		},
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(_ *cobra.Command, posArgs []string) error {
			if keyPath == "" || certPath == "" {
				return usageErr{"--key and --cert are required"}
			}
			if output == "" {
				return usageErr{"-o/--output is required"}
			}

			cert, err := util.ReadCertFromFile(certPath)
			if err != nil {
				return fmt.Errorf("reading cert: %w", err)
			}
			key, err := util.ReadKeyFromFile(keyPath)
			if err != nil {
				return fmt.Errorf("reading key: %w", err)
			}

			in, err := os.Open(posArgs[0])
			if err != nil {
				return err
			}
			defer in.Close()

			bin, err := authenticode.Parse(in)
			if err != nil {
				return fmt.Errorf("parsing PE: %w", err)
			}
			if err := signPE(bin, key, cert); err != nil {
				return fmt.Errorf("signing: %w", err)
			}
			return os.WriteFile(output, bin.Bytes(), 0o644)
		},
	}

	cmd.Flags().StringVar(&keyPath, "key", "", "path to the signing private key (PEM or DER)")
	cmd.Flags().StringVar(&certPath, "cert", "", "path to the signing certificate (PEM or DER)")
	cmd.Flags().StringVarP(&output, "output", "o", "", "output PE image (required)")
	return cmd
}
