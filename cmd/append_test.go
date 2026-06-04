package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitNamePath(t *testing.T) {
	cases := []struct {
		in       string
		wantErr  bool
		wantName string
		wantPath string
	}{
		{"name=path", false, "name", "path"},
		{"name=a=b", false, "name", "a=b"}, // first '=' is the separator
		{"", true, "", ""},
		{"=onlyvalue", true, "", ""},
		{"nameonly=", true, "", ""},
	}
	for _, c := range cases {
		name, path, err := splitNamePath(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("splitNamePath(%q) err=%v wantErr=%v", c.in, err, c.wantErr)
			continue
		}
		if !c.wantErr && (name != c.wantName || path != c.wantPath) {
			t.Errorf("splitNamePath(%q) = (%q,%q), want (%q,%q)",
				c.in, name, path, c.wantName, c.wantPath)
		}
	}
}

// --- Happy paths -------------------------------------------------------------

func TestAppend_NamedShortcut(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "stub.efi")
	cmdline := filepath.Join(dir, "cmdline")
	out := filepath.Join(dir, "out.efi")
	writeFile(t, stub, buildMinimalPE(t))
	writeFile(t, cmdline, []byte("console=ttyS0"))

	var buf bytes.Buffer
	rc := run([]string{"append", "--cmdline", cmdline, "-o", out, stub}, &buf)
	if rc != 0 {
		t.Fatalf("rc=%d stderr=%q", rc, buf.String())
	}
	info, err := os.Stat(out)
	if err != nil || info.Size() == 0 {
		t.Fatalf("output not written: %v size=%d", err, info.Size())
	}
}

func TestAppend_AllNamedShortcuts(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "stub.efi")
	linux := filepath.Join(dir, "linux")
	initrd := filepath.Join(dir, "initrd")
	cmdline := filepath.Join(dir, "cmdline")
	osrel := filepath.Join(dir, "osrel")
	uname := filepath.Join(dir, "uname")
	out := filepath.Join(dir, "out.efi")
	writeFile(t, stub, buildMinimalPE(t))
	writeFile(t, linux, []byte("vmlinuz"))
	writeFile(t, initrd, []byte("cpio"))
	writeFile(t, cmdline, []byte("console=ttyS0"))
	writeFile(t, osrel, []byte("ID=test\n"))
	writeFile(t, uname, []byte("6.6.74"))

	var buf bytes.Buffer
	rc := run([]string{
		"append",
		"--linux", linux,
		"--initrd", initrd,
		"--cmdline", cmdline,
		"--osrel", osrel,
		"--uname", uname,
		"-o", out, stub,
	}, &buf)
	if rc != 0 {
		t.Fatalf("rc=%d stderr=%q", rc, buf.String())
	}
}

func TestAppend_GenericSection(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "stub.efi")
	splash := filepath.Join(dir, "splash")
	out := filepath.Join(dir, "out.efi")
	writeFile(t, stub, buildMinimalPE(t))
	writeFile(t, splash, []byte("BM..."))

	var buf bytes.Buffer
	rc := run([]string{"append", "--section", ".splash=" + splash, "-o", out, stub}, &buf)
	if rc != 0 {
		t.Fatalf("rc=%d stderr=%q", rc, buf.String())
	}
}

func TestAppend_LegacyAddSection(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "stub.efi")
	osrel := filepath.Join(dir, "os-release")
	out := filepath.Join(dir, "out.efi")
	writeFile(t, stub, buildMinimalPE(t))
	writeFile(t, osrel, []byte("ID=test\n"))

	var buf bytes.Buffer
	rc := run([]string{"append", "--add-section", ".osrel=" + osrel, "-o", out, stub}, &buf)
	if rc != 0 {
		t.Fatalf("rc=%d stderr=%q", rc, buf.String())
	}
}

// --- Usage errors (exit code 2) ---------------------------------------------

func TestAppend_UsageMissingPositional(t *testing.T) {
	var buf bytes.Buffer
	rc := run([]string{"append", "-o", "out.efi", "--cmdline", "/dev/null"}, &buf)
	if rc != 2 {
		t.Fatalf("rc=%d, want 2 (no positional)", rc)
	}
}

func TestAppend_UsageMissingOutput(t *testing.T) {
	var buf bytes.Buffer
	rc := run([]string{"append", "--section", "a=b", "input"}, &buf)
	if rc != 2 {
		t.Fatalf("rc=%d, want 2", rc)
	}
	if !strings.Contains(buf.String(), "output") {
		t.Errorf("stderr should mention --output, got %q", buf.String())
	}
}

func TestAppend_UsageNoSections(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "stub.efi")
	out := filepath.Join(dir, "out.efi")
	writeFile(t, stub, buildMinimalPE(t))
	var buf bytes.Buffer
	rc := run([]string{"append", "-o", out, stub}, &buf)
	if rc != 2 {
		t.Fatalf("rc=%d, want 2 (no sections)", rc)
	}
}

func TestAppend_FlagParseError(t *testing.T) {
	var buf bytes.Buffer
	if rc := run([]string{"append", "--unknown-flag"}, &buf); rc != 2 {
		t.Fatalf("rc=%d, want 2", rc)
	}
}

func TestAppend_BadSectionFormat(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "stub.efi")
	out := filepath.Join(dir, "out.efi")
	writeFile(t, stub, buildMinimalPE(t))
	var buf bytes.Buffer
	rc := run([]string{"append", "--section", "no-equals", "-o", out, stub}, &buf)
	if rc != 2 {
		t.Fatalf("rc=%d, want 2", rc)
	}
}

// --- Runtime errors (exit code 1) -------------------------------------------

func TestAppend_NamedPathReadError(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "stub.efi")
	out := filepath.Join(dir, "out.efi")
	writeFile(t, stub, buildMinimalPE(t))
	var buf bytes.Buffer
	rc := run([]string{
		"append",
		"--linux", filepath.Join(dir, "no-such-file"),
		"-o", out, stub,
	}, &buf)
	if rc != 1 {
		t.Fatalf("rc=%d, want 1", rc)
	}
}

func TestAppend_SectionPathReadError(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "stub.efi")
	out := filepath.Join(dir, "out.efi")
	writeFile(t, stub, buildMinimalPE(t))
	var buf bytes.Buffer
	rc := run([]string{
		"append",
		"--section", ".x=" + filepath.Join(dir, "no-such-section-file"),
		"-o", out, stub,
	}, &buf)
	if rc != 1 {
		t.Fatalf("rc=%d, want 1", rc)
	}
}

func TestAppend_InputReadError(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.efi")
	osrel := filepath.Join(dir, "os-release")
	writeFile(t, osrel, []byte("ID=test\n"))
	var buf bytes.Buffer
	rc := run([]string{
		"append",
		"--osrel", osrel,
		"-o", out,
		filepath.Join(dir, "does-not-exist"),
	}, &buf)
	if rc != 1 {
		t.Fatalf("rc=%d, want 1", rc)
	}
}

func TestAppend_PeAppendError(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "stub.bin")
	out := filepath.Join(dir, "out.efi")
	osrel := filepath.Join(dir, "os-release")
	writeFile(t, stub, []byte("not a pe"))
	writeFile(t, osrel, []byte("ID=test\n"))
	var buf bytes.Buffer
	rc := run([]string{"append", "--osrel", osrel, "-o", out, stub}, &buf)
	if rc != 1 {
		t.Fatalf("rc=%d, want 1 (pe.Append should reject a non-PE input)", rc)
	}
}

func TestAppend_OutputWriteError(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "stub.efi")
	osrel := filepath.Join(dir, "os-release")
	writeFile(t, stub, buildMinimalPE(t))
	writeFile(t, osrel, []byte("ID=test\n"))
	// Nested non-existent directory → os.WriteFile fails.
	out := filepath.Join(dir, "nope", "deeper", "out.efi")
	var buf bytes.Buffer
	rc := run([]string{"append", "--osrel", osrel, "-o", out, stub}, &buf)
	if rc != 1 {
		t.Fatalf("rc=%d, want 1", rc)
	}
}
