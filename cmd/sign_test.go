package cmd

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/foxboron/go-uefi/authenticode"
)

// genTestKeypair writes a fresh RSA key + self-signed cert to keyPath/certPath
// in PEM form. Used by every sign test that needs valid credentials on disk.
func genTestKeypair(t *testing.T, keyPath, certPath string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "go-coff pec test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning},
		IsCA:         true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	writeFile(t, certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey: %v", err)
	}
	writeFile(t, keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
}

func TestSign_Success(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.efi")
	out := filepath.Join(dir, "out.efi")
	key := filepath.Join(dir, "key.pem")
	cert := filepath.Join(dir, "cert.pem")
	writeFile(t, in, buildMinimalPE(t))
	genTestKeypair(t, key, cert)

	var buf bytes.Buffer
	rc := run([]string{"sign", "--key", key, "--cert", cert, "-o", out, in}, &buf)
	if rc != 0 {
		t.Fatalf("rc=%d stderr=%q", rc, buf.String())
	}
	// The signed PE must be strictly larger than the input — the
	// attribute certificate table got appended.
	inInfo, _ := os.Stat(in)
	outInfo, err := os.Stat(out)
	if err != nil {
		t.Fatalf("output not written: %v", err)
	}
	if outInfo.Size() <= inInfo.Size() {
		t.Fatalf("signed PE size %d not larger than input %d (no signature appended?)",
			outInfo.Size(), inInfo.Size())
	}
}

// --- Usage errors (exit 2) ---------------------------------------------------

func TestSign_UsageMissingKey(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.efi")
	out := filepath.Join(dir, "out.efi")
	cert := filepath.Join(dir, "cert.pem")
	writeFile(t, in, buildMinimalPE(t))
	writeFile(t, cert, []byte("not used"))
	var buf bytes.Buffer
	rc := run([]string{"sign", "--cert", cert, "-o", out, in}, &buf)
	if rc != 2 {
		t.Fatalf("rc=%d, want 2", rc)
	}
}

func TestSign_UsageMissingCert(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.efi")
	out := filepath.Join(dir, "out.efi")
	key := filepath.Join(dir, "key.pem")
	writeFile(t, in, buildMinimalPE(t))
	writeFile(t, key, []byte("not used"))
	var buf bytes.Buffer
	rc := run([]string{"sign", "--key", key, "-o", out, in}, &buf)
	if rc != 2 {
		t.Fatalf("rc=%d, want 2", rc)
	}
}

func TestSign_UsageMissingOutput(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.efi")
	key := filepath.Join(dir, "key.pem")
	cert := filepath.Join(dir, "cert.pem")
	writeFile(t, in, buildMinimalPE(t))
	genTestKeypair(t, key, cert)
	var buf bytes.Buffer
	rc := run([]string{"sign", "--key", key, "--cert", cert, in}, &buf)
	if rc != 2 {
		t.Fatalf("rc=%d, want 2", rc)
	}
}

func TestSign_UsageNoArgs(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "key.pem")
	cert := filepath.Join(dir, "cert.pem")
	genTestKeypair(t, key, cert)
	var buf bytes.Buffer
	rc := run([]string{"sign", "--key", key, "--cert", cert, "-o", "out.efi"}, &buf)
	if rc != 2 {
		t.Fatalf("rc=%d, want 2 (no positional)", rc)
	}
}

// --- Runtime errors (exit 1) -------------------------------------------------

func TestSign_BadCertFile(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.efi")
	out := filepath.Join(dir, "out.efi")
	key := filepath.Join(dir, "key.pem")
	cert := filepath.Join(dir, "cert.pem")
	writeFile(t, in, buildMinimalPE(t))
	writeFile(t, cert, []byte("not a PEM cert"))
	writeFile(t, key, []byte("not used"))
	var buf bytes.Buffer
	rc := run([]string{"sign", "--key", key, "--cert", cert, "-o", out, in}, &buf)
	if rc != 1 {
		t.Fatalf("rc=%d, want 1", rc)
	}
}

func TestSign_BadKeyFile(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.efi")
	out := filepath.Join(dir, "out.efi")
	key := filepath.Join(dir, "key.pem")
	cert := filepath.Join(dir, "cert.pem")
	writeFile(t, in, buildMinimalPE(t))
	// Generate a valid cert, then trash the key.
	genTestKeypair(t, key, cert)
	writeFile(t, key, []byte("not a PEM key"))
	var buf bytes.Buffer
	rc := run([]string{"sign", "--key", key, "--cert", cert, "-o", out, in}, &buf)
	if rc != 1 {
		t.Fatalf("rc=%d, want 1", rc)
	}
}

func TestSign_InputOpenError(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.efi")
	key := filepath.Join(dir, "key.pem")
	cert := filepath.Join(dir, "cert.pem")
	genTestKeypair(t, key, cert)
	var buf bytes.Buffer
	rc := run([]string{
		"sign", "--key", key, "--cert", cert, "-o", out,
		filepath.Join(dir, "does-not-exist"),
	}, &buf)
	if rc != 1 {
		t.Fatalf("rc=%d, want 1", rc)
	}
}

func TestSign_ParsePEError(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.efi")
	out := filepath.Join(dir, "out.efi")
	key := filepath.Join(dir, "key.pem")
	cert := filepath.Join(dir, "cert.pem")
	writeFile(t, in, []byte("not a PE"))
	genTestKeypair(t, key, cert)
	var buf bytes.Buffer
	rc := run([]string{"sign", "--key", key, "--cert", cert, "-o", out, in}, &buf)
	if rc != 1 {
		t.Fatalf("rc=%d, want 1", rc)
	}
}

// TestSign_SignError exercises the signing-error branch via the signPE
// injection point. The underlying authenticode.Sign almost never fails on
// valid RSA inputs, so we substitute a stub that returns a forced error to
// keep the branch covered without inventing pathological credentials.
func TestSign_SignError(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.efi")
	out := filepath.Join(dir, "out.efi")
	key := filepath.Join(dir, "key.pem")
	cert := filepath.Join(dir, "cert.pem")
	writeFile(t, in, buildMinimalPE(t))
	genTestKeypair(t, key, cert)

	old := signPE
	defer func() { signPE = old }()
	signPE = func(_ *authenticode.PECOFFBinary, _ crypto.Signer, _ *x509.Certificate) error {
		return errForced
	}

	var buf bytes.Buffer
	rc := run([]string{"sign", "--key", key, "--cert", cert, "-o", out, in}, &buf)
	if rc != 1 {
		t.Fatalf("rc=%d, want 1", rc)
	}
}

var errForced = errSentinel("forced sign failure")

type errSentinel string

func (e errSentinel) Error() string { return string(e) }

func TestSign_OutputWriteError(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.efi")
	key := filepath.Join(dir, "key.pem")
	cert := filepath.Join(dir, "cert.pem")
	writeFile(t, in, buildMinimalPE(t))
	genTestKeypair(t, key, cert)
	out := filepath.Join(dir, "nope", "deeper", "out.efi")
	var buf bytes.Buffer
	rc := run([]string{"sign", "--key", key, "--cert", cert, "-o", out, in}, &buf)
	if rc != 1 {
		t.Fatalf("rc=%d, want 1", rc)
	}
}
