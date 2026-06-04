package cmd

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// minimalCOFF builds a tiny COFF/PE relocatable object with a single
// `.text` section containing a `_start` symbol at offset 0.
//
// debug/pe.NewFile unconditionally reads the first 96 bytes of the file
// (to check the DOS-header MZ signature), so the buffer is padded out to
// that minimum regardless of the actual structural size.
func minimalCOFF(t *testing.T, machine uint16) []byte {
	t.Helper()
	const (
		hdrSize = 20
		secSize = 40
		symSize = 18
		minRead = 96 // debug/pe.NewFile reads this many bytes at offset 0
	)
	text := []byte{0x90, 0x90, 0x90, 0x90, 0xC3, 0x00, 0x00, 0x00}
	bufSize := hdrSize + secSize + len(text) + symSize + 4
	if bufSize < minRead {
		bufSize = minRead
	}
	buf := make([]byte, bufSize)
	// COFF file header
	binary.LittleEndian.PutUint16(buf[0:], machine)
	binary.LittleEndian.PutUint16(buf[2:], 1)                                 // NumberOfSections
	binary.LittleEndian.PutUint32(buf[8:], hdrSize+secSize+uint32(len(text))) // PointerToSymbolTable
	binary.LittleEndian.PutUint32(buf[12:], 1)                                // NumberOfSymbols
	binary.LittleEndian.PutUint16(buf[16:], 0)                                // SizeOfOptionalHeader
	// Section header (".text")
	sec := hdrSize
	copy(buf[sec:sec+8], []byte(".text"))
	binary.LittleEndian.PutUint32(buf[sec+8:], 0)                  // VirtualSize
	binary.LittleEndian.PutUint32(buf[sec+12:], 0)                 // VirtualAddress
	binary.LittleEndian.PutUint32(buf[sec+16:], uint32(len(text))) // SizeOfRawData
	binary.LittleEndian.PutUint32(buf[sec+20:], hdrSize+secSize)   // PointerToRawData
	binary.LittleEndian.PutUint32(buf[sec+36:], 0x60000020)        // CODE|MEM_EXECUTE|MEM_READ
	// Section data
	copy(buf[sec+secSize:], text)
	// Symbol table: one symbol _start, defined in section 1 at offset 0.
	sym := sec + secSize + len(text)
	copy(buf[sym:sym+8], []byte("_start"))
	binary.LittleEndian.PutUint32(buf[sym+8:], 0)  // Value
	binary.LittleEndian.PutUint16(buf[sym+12:], 1) // SectionNumber (1-based)
	binary.LittleEndian.PutUint16(buf[sym+14:], 0) // Type
	buf[sym+16] = 2                                // StorageClass = IMAGE_SYM_CLASS_EXTERNAL
	buf[sym+17] = 0                                // NumberOfAuxSymbols
	// String table: 4-byte size, no entries.
	binary.LittleEndian.PutUint32(buf[sym+symSize:], 4)
	return buf
}

// --- Happy path -------------------------------------------------------------

func TestLink_AMD64(t *testing.T) {
	dir := t.TempDir()
	obj := filepath.Join(dir, "min-amd64.o")
	out := filepath.Join(dir, "out.efi")
	writeFile(t, obj, minimalCOFF(t, 0x8664))

	var buf bytes.Buffer
	rc := run([]string{"link", "-o", out, obj}, &buf)
	if rc != 0 {
		t.Fatalf("rc=%d stderr=%q", rc, buf.String())
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	pf, err := pe.NewFile(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("output not a PE: %v", err)
	}
	defer pf.Close()
	if pf.OptionalHeader == nil {
		t.Fatal("output has no optional header")
	}
	oh, _ := pf.OptionalHeader.(*pe.OptionalHeader64)
	if oh == nil {
		t.Fatal("output is not PE32+")
	}
	if oh.Subsystem != 10 {
		t.Errorf("Subsystem = %d, want 10", oh.Subsystem)
	}
}

func TestLink_ExplicitMachine(t *testing.T) {
	dir := t.TempDir()
	obj := filepath.Join(dir, "min.o")
	out := filepath.Join(dir, "out.efi")
	writeFile(t, obj, minimalCOFF(t, 0x8664))
	var buf bytes.Buffer
	rc := run([]string{"link", "--machine", "amd64", "-o", out, obj}, &buf)
	if rc != 0 {
		t.Fatalf("rc=%d stderr=%q", rc, buf.String())
	}
}

func TestLink_AllOptionFlags(t *testing.T) {
	dir := t.TempDir()
	obj := filepath.Join(dir, "min.o")
	out := filepath.Join(dir, "out.efi")
	writeFile(t, obj, minimalCOFF(t, 0x8664))
	var buf bytes.Buffer
	rc := run([]string{
		"link",
		"--entry", "_start",
		"--subsystem", "10",
		"--image-base", "0x10000",
		"--allow-unresolved",
		"--section-alignment", "4096",
		"--file-alignment", "512",
		"--header-reserve", "1024",
		"-o", out, obj,
	}, &buf)
	if rc != 0 {
		t.Fatalf("rc=%d stderr=%q", rc, buf.String())
	}
}

func TestLink_StubObjects(t *testing.T) {
	// Use the cloud-boot stub objects if available; otherwise skip.
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			main := "../../../go-coff/stub/main-" + arch + ".o"
			thunk := "../../../go-coff/stub/thunk-" + arch + ".o"
			if _, err := os.Stat(main); err != nil {
				t.Skipf("no fixture %s", main)
			}
			if _, err := os.Stat(thunk); err != nil {
				t.Skipf("no fixture %s", thunk)
			}
			dir := t.TempDir()
			out := filepath.Join(dir, "out.efi")
			var buf bytes.Buffer
			rc := run([]string{"link", "--allow-unresolved", "-o", out, main, thunk}, &buf)
			if rc != 0 {
				t.Fatalf("rc=%d stderr=%q", rc, buf.String())
			}
		})
	}
}

// --- Usage errors (exit code 2) ---------------------------------------------

func TestLink_UsageMissingPositional(t *testing.T) {
	var buf bytes.Buffer
	if rc := run([]string{"link", "-o", "out.efi"}, &buf); rc != 2 {
		t.Fatalf("rc=%d, want 2", rc)
	}
}

func TestLink_UsageMissingOutput(t *testing.T) {
	dir := t.TempDir()
	obj := filepath.Join(dir, "x.o")
	writeFile(t, obj, minimalCOFF(t, 0x8664))
	var buf bytes.Buffer
	rc := run([]string{"link", obj}, &buf)
	if rc != 2 {
		t.Fatalf("rc=%d, want 2", rc)
	}
	if !strings.Contains(buf.String(), "output") {
		t.Errorf("stderr should mention --output, got %q", buf.String())
	}
}

func TestLink_UsageBadMachine(t *testing.T) {
	dir := t.TempDir()
	obj := filepath.Join(dir, "x.o")
	writeFile(t, obj, minimalCOFF(t, 0x8664))
	var buf bytes.Buffer
	rc := run([]string{"link", "--machine", "z80", "-o", "out.efi", obj}, &buf)
	if rc != 2 {
		t.Fatalf("rc=%d, want 2", rc)
	}
}

func TestLink_UsageBadImageBase(t *testing.T) {
	dir := t.TempDir()
	obj := filepath.Join(dir, "x.o")
	writeFile(t, obj, minimalCOFF(t, 0x8664))
	var buf bytes.Buffer
	rc := run([]string{"link", "--image-base", "not-a-number", "-o", "out.efi", obj}, &buf)
	if rc != 2 {
		t.Fatalf("rc=%d, want 2", rc)
	}
}

func TestLink_UsageFlagParseError(t *testing.T) {
	var buf bytes.Buffer
	if rc := run([]string{"link", "--no-such-flag"}, &buf); rc != 2 {
		t.Fatalf("rc=%d, want 2", rc)
	}
}

// --- Runtime errors (exit code 1) -------------------------------------------

func TestLink_InputReadError(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.efi")
	var buf bytes.Buffer
	rc := run([]string{"link", "-o", out, filepath.Join(dir, "missing.o")}, &buf)
	if rc != 1 {
		t.Fatalf("rc=%d, want 1", rc)
	}
}

func TestLink_InputNotAnObject(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "garbage.o")
	out := filepath.Join(dir, "out.efi")
	writeFile(t, bad, []byte("not coff"))
	var buf bytes.Buffer
	rc := run([]string{"link", "-o", out, bad}, &buf)
	if rc != 1 {
		t.Fatalf("rc=%d, want 1", rc)
	}
}

func TestLink_LinkerError(t *testing.T) {
	// _start defined-but-renamed: the linker will fail to resolve the
	// entry symbol and surface that as a runtime error.
	dir := t.TempDir()
	obj := filepath.Join(dir, "x.o")
	out := filepath.Join(dir, "out.efi")
	writeFile(t, obj, minimalCOFF(t, 0x8664))
	var buf bytes.Buffer
	rc := run([]string{"link", "--entry", "no_such_symbol", "-o", out, obj}, &buf)
	if rc != 1 {
		t.Fatalf("rc=%d, want 1", rc)
	}
}

func TestLink_OutputWriteError(t *testing.T) {
	dir := t.TempDir()
	obj := filepath.Join(dir, "x.o")
	writeFile(t, obj, minimalCOFF(t, 0x8664))
	out := filepath.Join(dir, "no", "such", "dir", "out.efi")
	var buf bytes.Buffer
	rc := run([]string{"link", "-o", out, obj}, &buf)
	if rc != 1 {
		t.Fatalf("rc=%d, want 1", rc)
	}
}

// --- internal helpers -------------------------------------------------------

func TestMachineFromName_Table(t *testing.T) {
	cases := []struct {
		in   string
		want uint16
		bad  bool
	}{
		{"", 0, false},
		{"amd64", 0x8664, false},
		{"x86_64", 0x8664, false},
		{"x64", 0x8664, false},
		{"arm64", 0xaa64, false},
		{"aarch64", 0xaa64, false},
		{"riscv64", 0x5064, false},
		{"loongarch64", 0x6264, false},
		{"loong64", 0x6264, false},
		{"sparc", 0, true},
	}
	for _, c := range cases {
		got, err := machineFromName(c.in)
		if (err != nil) != c.bad {
			t.Errorf("machineFromName(%q) err=%v bad=%v", c.in, err, c.bad)
		}
		if !c.bad && got != c.want {
			t.Errorf("machineFromName(%q) = 0x%x, want 0x%x", c.in, got, c.want)
		}
	}
}

func TestParseUint_DecimalAndHex(t *testing.T) {
	v, err := parseUint("16384", 64)
	if err != nil || v != 16384 {
		t.Errorf("decimal: %d / %v", v, err)
	}
	v, err = parseUint("0x4000", 64)
	if err != nil || v != 0x4000 {
		t.Errorf("hex lower: %d / %v", v, err)
	}
	v, err = parseUint("0X10", 64)
	if err != nil || v != 0x10 {
		t.Errorf("hex upper: %d / %v", v, err)
	}
	if _, err := parseUint("nope", 64); err == nil {
		t.Errorf("expected error for non-numeric input")
	}
}
