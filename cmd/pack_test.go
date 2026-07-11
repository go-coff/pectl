// Copyright (c) 2026 The go-coff Authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that
// can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildMinimalPE in root_test.go produces a tiny PE32+ image with
// Machine = 0x8664 (amd64) — efipack's InferArch happily accepts it.
// Since this PR3 path packs amd64 inputs using the embedded amd64
// stub blob (which round-trips at the host-side regardless of the
// "amd64 runtime deferred" status), we use that as the test input.

// --- happy path ---------------------------------------------------------------

func TestPack_Default(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.efi")
	out := filepath.Join(dir, "out.efi")
	writeFile(t, in, buildMinimalPE(t))

	var buf bytes.Buffer
	rc := run([]string{"pack", "-o", out, in}, &buf)
	if rc != 0 {
		t.Fatalf("rc=%d stderr=%q", rc, buf.String())
	}
	// The one-line summary contains the inputs, outputs, and the
	// compressor name. Hard-coded "compressor=flate" is the contract
	// other tools grep against.
	s := buf.String()
	for _, want := range []string{in, out, "compressor=flate", "reduction"} {
		if !strings.Contains(s, want) {
			t.Errorf("summary missing %q in %q", want, s)
		}
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(raw, []byte("MZ")) {
		t.Errorf("output is not a PE")
	}
}

func TestPack_ExplicitFlateLevel(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.efi")
	out := filepath.Join(dir, "out.efi")
	writeFile(t, in, buildMinimalPE(t))

	var buf bytes.Buffer
	rc := run([]string{"pack", "-c", "flate", "--level", "9", "-o", out, in}, &buf)
	if rc != 0 {
		t.Fatalf("rc=%d stderr=%q", rc, buf.String())
	}
}

// --- usage errors (exit 2) ----------------------------------------------------

func TestPack_UsageMissingOutput(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.efi")
	writeFile(t, in, buildMinimalPE(t))
	var buf bytes.Buffer
	rc := run([]string{"pack", in}, &buf)
	if rc != 2 {
		t.Fatalf("rc=%d, want 2", rc)
	}
	if !strings.Contains(buf.String(), "output") {
		t.Errorf("stderr should mention --output, got %q", buf.String())
	}
}

func TestPack_UsageWrongArgCount(t *testing.T) {
	var buf bytes.Buffer
	if rc := run([]string{"pack", "-o", "out.efi"}, &buf); rc != 2 {
		t.Fatalf("rc=%d, want 2", rc)
	}
}

func TestPack_UsageBadCompressor(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.efi")
	writeFile(t, in, buildMinimalPE(t))
	var buf bytes.Buffer
	rc := run([]string{"pack", "-c", "snappy", "-o", filepath.Join(dir, "out.efi"), in}, &buf)
	if rc != 2 {
		t.Fatalf("rc=%d, want 2", rc)
	}
	if !strings.Contains(buf.String(), "snappy") {
		t.Errorf("stderr should mention the bad codec name, got %q", buf.String())
	}
}

func TestPack_UsageBadLevel(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.efi")
	writeFile(t, in, buildMinimalPE(t))
	var buf bytes.Buffer
	rc := run([]string{"pack", "--level", "42", "-o", filepath.Join(dir, "out.efi"), in}, &buf)
	if rc != 2 {
		t.Fatalf("rc=%d, want 2", rc)
	}
	if !strings.Contains(buf.String(), "level") {
		t.Errorf("stderr should mention level, got %q", buf.String())
	}
}

func TestPack_UsageFlagParseError(t *testing.T) {
	var buf bytes.Buffer
	if rc := run([]string{"pack", "--no-such-flag"}, &buf); rc != 2 {
		t.Fatalf("rc=%d, want 2", rc)
	}
}

// --- runtime errors (exit 1) --------------------------------------------------

func TestPack_InputReadError(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.efi")
	var buf bytes.Buffer
	rc := run([]string{"pack", "-o", out, filepath.Join(dir, "missing.efi")}, &buf)
	if rc != 1 {
		t.Fatalf("rc=%d, want 1", rc)
	}
}

func TestPack_InputNotPE(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "garbage.efi")
	out := filepath.Join(dir, "out.efi")
	writeFile(t, bad, []byte("not a PE at all"))
	var buf bytes.Buffer
	rc := run([]string{"pack", "-o", out, bad}, &buf)
	if rc != 1 {
		t.Fatalf("rc=%d, want 1", rc)
	}
}

func TestPack_OutputWriteError(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.efi")
	writeFile(t, in, buildMinimalPE(t))
	bad := filepath.Join(dir, "no", "such", "dir", "out.efi")
	var buf bytes.Buffer
	rc := run([]string{"pack", "-o", bad, in}, &buf)
	if rc != 1 {
		t.Fatalf("rc=%d, want 1", rc)
	}
}

// TestPack_LZFSEEmitsWarningButSucceeds asserts that -c lzfse now
// produces a packed PE on disk (the host-side codec works) while
// surfacing a WARNING that the resulting binary isn't yet runnable
// under firmware (the embedded stub is flate-only). Replaces the
// pre-M6.2-PR4 expectation that -c lzfse returned a not-implemented
// error.
func TestPack_LZFSEEmitsWarningButSucceeds(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.efi")
	out := filepath.Join(dir, "out.efi")
	writeFile(t, in, buildMinimalPE(t))
	var buf bytes.Buffer
	rc := run([]string{"pack", "-c", "lzfse", "-o", out, in}, &buf)
	if rc != 0 {
		t.Fatalf("rc=%d stderr=%q, want 0", rc, buf.String())
	}
	s := buf.String()
	if !strings.Contains(s, "WARNING") || !strings.Contains(s, "lzfse") {
		t.Errorf("stderr should surface an lzfse WARNING, got %q", s)
	}
	for _, want := range []string{"compressor=lzfse", "reduction"} {
		if !strings.Contains(s, want) {
			t.Errorf("summary missing %q in %q", want, s)
		}
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(raw, []byte("MZ")) {
		t.Errorf("output is not a PE")
	}
}

// TestPack_LZ4EmitsWarningButSucceeds asserts that -c lz4 now produces
// a packed PE on disk (the pure-Go go-compressions/lz4 host codec works,
// wired in efipack v0.3.0) while surfacing a WARNING that the resulting
// binary isn't yet runnable under firmware (the embedded stub decodes
// FLAT only). Replaces the pre-v0.3.0 "lz4 not implemented" expectation.
func TestPack_LZ4EmitsWarningButSucceeds(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.efi")
	out := filepath.Join(dir, "out.efi")
	writeFile(t, in, buildMinimalPE(t))
	var buf bytes.Buffer
	rc := run([]string{"pack", "-c", "lz4", "-o", out, in}, &buf)
	if rc != 0 {
		t.Fatalf("rc=%d stderr=%q, want 0", rc, buf.String())
	}
	s := buf.String()
	if !strings.Contains(s, "WARNING") || !strings.Contains(s, "lz4") {
		t.Errorf("stderr should surface an lz4 WARNING, got %q", s)
	}
	for _, want := range []string{"compressor=lz4", "reduction"} {
		if !strings.Contains(s, want) {
			t.Errorf("summary missing %q in %q", want, s)
		}
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(raw, []byte("MZ")) {
		t.Errorf("output is not a PE")
	}
}

// --- internal helper -----------------------------------------------------------

func TestParseCompressorName(t *testing.T) {
	cases := []struct {
		name string
		want string // empty = wantErr
	}{
		{"flate", "flate"},
		{"lzfse", "lzfse"},
		{"lz4", "lz4"},
		{"", ""},
		{"gzip", ""},
	}
	for _, tc := range cases {
		c, err := parseCompressorName(tc.name)
		if tc.want == "" {
			if err == nil {
				t.Errorf("parseCompressorName(%q): want error", tc.name)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseCompressorName(%q): %v", tc.name, err)
			continue
		}
		if c.String() != tc.want {
			t.Errorf("parseCompressorName(%q) = %s, want %s", tc.name, c.String(), tc.want)
		}
	}
}
