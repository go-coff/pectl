package main

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// buildMinimalPE returns a tiny but well-formed PE32+ image that pe.Append
// can extend. It is the same construction used by the pe package's own tests,
// inlined here so the pec module stays self-contained.
func buildMinimalPE(t *testing.T) []byte {
	t.Helper()
	const (
		dosSize       = 0x40
		optSize       = 240
		secTableSlots = 8
		fileAlign     = 512
		sectionAlign  = 0x1000
	)
	alignUp := func(v, a uint32) uint32 {
		if r := v % a; r != 0 {
			return v + (a - r)
		}
		return v
	}
	headerEnd := dosSize + 4 + 20 + optSize + secTableSlots*40
	sizeOfHeaders := alignUp(uint32(headerEnd), fileAlign)

	textData := bytes.Repeat([]byte{0x90}, 16)
	textRaw := alignUp(uint32(len(textData)), fileAlign)
	textVA := uint32(sectionAlign)

	buf := make([]byte, sizeOfHeaders+textRaw)
	buf[0] = 'M'
	buf[1] = 'Z'
	binary.LittleEndian.PutUint32(buf[0x3C:], dosSize)
	copy(buf[dosSize:dosSize+4], []byte("PE\x00\x00"))
	coffOff := dosSize + 4
	binary.LittleEndian.PutUint16(buf[coffOff+0:], 0x8664)
	binary.LittleEndian.PutUint16(buf[coffOff+2:], 1)
	binary.LittleEndian.PutUint16(buf[coffOff+16:], optSize)
	binary.LittleEndian.PutUint16(buf[coffOff+18:], 0x002E)
	optOff := coffOff + 20
	binary.LittleEndian.PutUint16(buf[optOff+0:], 0x020B)
	binary.LittleEndian.PutUint32(buf[optOff+32:], sectionAlign)
	binary.LittleEndian.PutUint32(buf[optOff+36:], fileAlign)
	binary.LittleEndian.PutUint32(buf[optOff+56:], textVA+alignUp(uint32(len(textData)), sectionAlign))
	binary.LittleEndian.PutUint32(buf[optOff+60:], sizeOfHeaders)
	binary.LittleEndian.PutUint32(buf[optOff+108:], 16)
	secOff := optOff + optSize
	copy(buf[secOff:secOff+8], []byte(".text"))
	binary.LittleEndian.PutUint32(buf[secOff+8:], uint32(len(textData)))
	binary.LittleEndian.PutUint32(buf[secOff+12:], textVA)
	binary.LittleEndian.PutUint32(buf[secOff+16:], textRaw)
	binary.LittleEndian.PutUint32(buf[secOff+20:], sizeOfHeaders)
	binary.LittleEndian.PutUint32(buf[secOff+36:], 0x60000020)
	copy(buf[sizeOfHeaders:], textData)
	return buf
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("writeFile %s: %v", path, err)
	}
}

func TestAddList_Set(t *testing.T) {
	cases := []struct {
		in      string
		wantErr bool
	}{
		{"name=path", false},
		{"", true},
		{"=onlyvalue", true},
		{"nameonly=", true},
	}
	for _, c := range cases {
		var l addList
		err := l.Set(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("Set(%q) err=%v wantErr=%v", c.in, err, c.wantErr)
		}
	}
}

func TestAddList_String(t *testing.T) {
	var l addList
	if l.String() != "[]" {
		t.Errorf("empty String() = %q", l.String())
	}
	_ = l.Set("a=b")
	if l.String() == "" {
		t.Error("populated String() should be non-empty")
	}
}

func TestRun_Success(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "stub.efi")
	osrel := filepath.Join(dir, "os-release")
	out := filepath.Join(dir, "out.efi")
	writeFile(t, stub, buildMinimalPE(t))
	writeFile(t, osrel, []byte("ID=test\n"))

	var buf bytes.Buffer
	rc := run([]string{"--add-section", ".osrel=" + osrel, "-o", out, stub}, &buf)
	if rc != 0 {
		t.Fatalf("rc=%d stderr=%q", rc, buf.String())
	}
	info, err := os.Stat(out)
	if err != nil || info.Size() == 0 {
		t.Fatalf("output not written: %v size=%d", err, info.Size())
	}
}

func TestRun_UsageNoArgs(t *testing.T) {
	var buf bytes.Buffer
	if rc := run(nil, &buf); rc != 2 {
		t.Fatalf("rc=%d, want 2", rc)
	}
}

func TestRun_UsageMissingOutput(t *testing.T) {
	var buf bytes.Buffer
	if rc := run([]string{"--add-section", "a=b", "input"}, &buf); rc != 2 {
		t.Fatalf("rc=%d, want 2", rc)
	}
}

func TestRun_FlagParseError(t *testing.T) {
	var buf bytes.Buffer
	if rc := run([]string{"--unknown-flag"}, &buf); rc != 2 {
		t.Fatalf("rc=%d, want 2", rc)
	}
}

func TestRun_InputReadError(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.efi")
	missingSection := filepath.Join(dir, "missing-section")
	writeFile(t, missingSection, []byte("x"))
	var buf bytes.Buffer
	rc := run([]string{
		"--add-section", ".x=" + missingSection,
		"-o", out,
		filepath.Join(dir, "does-not-exist"),
	}, &buf)
	if rc != 1 {
		t.Fatalf("rc=%d, want 1", rc)
	}
}

func TestRun_SectionReadError(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "stub.efi")
	out := filepath.Join(dir, "out.efi")
	writeFile(t, stub, buildMinimalPE(t))
	var buf bytes.Buffer
	rc := run([]string{
		"--add-section", ".x=" + filepath.Join(dir, "no-such-section-file"),
		"-o", out,
		stub,
	}, &buf)
	if rc != 1 {
		t.Fatalf("rc=%d, want 1", rc)
	}
}

func TestRun_AppendError(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "stub.bin")
	section := filepath.Join(dir, "data")
	out := filepath.Join(dir, "out.efi")
	writeFile(t, stub, []byte("not a pe"))
	writeFile(t, section, []byte("payload"))
	var buf bytes.Buffer
	rc := run([]string{
		"--add-section", ".x=" + section,
		"-o", out,
		stub,
	}, &buf)
	if rc != 1 {
		t.Fatalf("rc=%d, want 1 (pe.Append should reject a non-PE input)", rc)
	}
}

func TestRun_OutputWriteError(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "stub.efi")
	section := filepath.Join(dir, "data")
	writeFile(t, stub, buildMinimalPE(t))
	writeFile(t, section, []byte("payload"))
	// Write to a path nested inside a non-existent directory → os.WriteFile fails.
	out := filepath.Join(dir, "nope", "deeper", "out.efi")
	var buf bytes.Buffer
	rc := run([]string{
		"--add-section", ".x=" + section,
		"-o", out,
		stub,
	}, &buf)
	if rc != 1 {
		t.Fatalf("rc=%d, want 1", rc)
	}
}

func TestMain_Entry(t *testing.T) {
	// Drive main() with osExit and os.Args intercepted, so its single statement
	// is covered without actually terminating the test process.
	oldExit, oldArgs := osExit, os.Args
	defer func() { osExit = oldExit; os.Args = oldArgs }()

	var gotCode int
	osExit = func(c int) { gotCode = c; panic("exit") }
	os.Args = []string{"pec"} // no flags → run() returns 2

	// Swallow flag's stderr noise.
	saved := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	defer func() {
		os.Stderr = saved
		w.Close()
		_, _ = io.Copy(io.Discard, r)
	}()

	func() {
		defer func() { _ = recover() }()
		main()
	}()
	if gotCode != 2 {
		t.Fatalf("exit code = %d, want 2", gotCode)
	}
}
