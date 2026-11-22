package cmd

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-binfmt/fwimg"
)

// buildELF makes a minimal ELF64 executable with one PT_LOAD segment (file
// content) at the given paddr, plus an entry point — enough for objcopy.
func buildELF(t *testing.T, machine elf.Machine, entry, paddr uint64, data []byte) []byte {
	t.Helper()
	const phoff, segOff = 64, 0x200
	buf := make([]byte, segOff+len(data)+16)
	copy(buf, []byte{0x7f, 'E', 'L', 'F'})
	buf[4], buf[5], buf[6] = 2, 1, 1
	binary.LittleEndian.PutUint16(buf[16:], uint16(elf.ET_EXEC))
	binary.LittleEndian.PutUint16(buf[18:], uint16(machine))
	binary.LittleEndian.PutUint32(buf[20:], 1)
	binary.LittleEndian.PutUint64(buf[24:], entry)
	binary.LittleEndian.PutUint64(buf[32:], phoff)
	binary.LittleEndian.PutUint16(buf[52:], 64)
	binary.LittleEndian.PutUint16(buf[54:], 56)
	binary.LittleEndian.PutUint16(buf[56:], 1)
	p := phoff
	binary.LittleEndian.PutUint32(buf[p:], uint32(elf.PT_LOAD))
	binary.LittleEndian.PutUint32(buf[p+4:], uint32(elf.PF_R|elf.PF_X))
	binary.LittleEndian.PutUint64(buf[p+8:], segOff)
	binary.LittleEndian.PutUint64(buf[p+16:], paddr) // vaddr
	binary.LittleEndian.PutUint64(buf[p+24:], paddr) // paddr
	binary.LittleEndian.PutUint64(buf[p+32:], uint64(len(data)))
	binary.LittleEndian.PutUint64(buf[p+40:], uint64(len(data)))
	binary.LittleEndian.PutUint64(buf[p+48:], 0x1000)
	copy(buf[segOff:], data)
	return buf
}

func writeELF(t *testing.T, dir string, machine elf.Machine) string {
	t.Helper()
	p := filepath.Join(dir, "in.elf")
	writeFile(t, p, buildELF(t, machine, 0x80004, 0x80000, []byte("BAREMETAL")))
	return p
}

// --- happy paths, one per output format --------------------------------------

func TestObjcopy_Formats(t *testing.T) {
	for _, f := range []string{"binary", "srec", "ihex", "uimage"} {
		t.Run(f, func(t *testing.T) {
			dir := t.TempDir()
			in := writeELF(t, dir, elf.EM_AARCH64)
			out := filepath.Join(dir, "out."+f)
			var buf bytes.Buffer
			rc := run([]string{"objcopy", "-O", f, "-o", out, in}, &buf)
			if rc != 0 {
				t.Fatalf("rc=%d stderr=%q", rc, buf.String())
			}
			b, err := os.ReadFile(out)
			if err != nil || len(b) == 0 {
				t.Fatalf("output: %v (len %d)", err, len(b))
			}
			switch f {
			case "binary":
				if string(b) != "BAREMETAL" {
					t.Errorf("binary = %q", b)
				}
			case "srec":
				if b[0] != 'S' {
					t.Error("not SREC")
				}
			case "ihex":
				if b[0] != ':' {
					t.Error("not IHEX")
				}
			case "uimage":
				if binary.BigEndian.Uint32(b) != 0x27051956 {
					t.Error("bad uImage magic")
				}
			}
		})
	}
}

func TestObjcopy_DefaultFormatIsBinary(t *testing.T) {
	dir := t.TempDir()
	in := writeELF(t, dir, elf.EM_AARCH64)
	out := filepath.Join(dir, "out.bin")
	var buf bytes.Buffer
	if rc := run([]string{"objcopy", "-o", out, in}, &buf); rc != 0 {
		t.Fatalf("rc=%d %q", rc, buf.String())
	}
	b, _ := os.ReadFile(out)
	if string(b) != "BAREMETAL" {
		t.Errorf("binary = %q", b)
	}
}

func TestObjcopy_AllOverrides(t *testing.T) {
	dir := t.TempDir()
	in := writeELF(t, dir, elf.EM_LOONGARCH)
	out := filepath.Join(dir, "uImage")
	var buf bytes.Buffer
	rc := run([]string{
		"objcopy", "-O", "uimage",
		"--load", "0x80000", "--entry", "0x80004", "--name", "linux",
		"--uimage-arch", "loongarch", "--uimage-os", "5", "--uimage-type", "2",
		"--use-vaddr", "--pad", "0xff",
		"-o", out, in,
	}, &buf)
	if rc != 0 {
		t.Fatalf("rc=%d %q", rc, buf.String())
	}
}

// uimage with no --uimage-arch derives the arch from the ELF machine.
func TestObjcopy_UImageArchFromELF(t *testing.T) {
	dir := t.TempDir()
	in := writeELF(t, dir, elf.EM_X86_64)
	out := filepath.Join(dir, "uImage")
	var buf bytes.Buffer
	if rc := run([]string{"objcopy", "-O", "uimage", "-o", out, in}, &buf); rc != 0 {
		t.Fatalf("rc=%d %q", rc, buf.String())
	}
	b, _ := os.ReadFile(out)
	if b[29] != fwimg.ArchX86_64 {
		t.Errorf("arch = %d, want %d", b[29], fwimg.ArchX86_64)
	}
}

// uimage with an ELF machine we don't map leaves arch 0 (UImage defaults it).
func TestObjcopy_UImageUnknownELFArch(t *testing.T) {
	dir := t.TempDir()
	in := writeELF(t, dir, elf.EM_MIPS)
	out := filepath.Join(dir, "uImage")
	var buf bytes.Buffer
	if rc := run([]string{"objcopy", "-O", "uimage", "-o", out, in}, &buf); rc != 0 {
		t.Fatalf("rc=%d %q", rc, buf.String())
	}
}

func TestObjcopy_InvalidPadFallsBackToZero(t *testing.T) {
	dir := t.TempDir()
	in := writeELF(t, dir, elf.EM_AARCH64)
	out := filepath.Join(dir, "out.bin")
	var buf bytes.Buffer
	if rc := run([]string{"objcopy", "--pad", "notnum", "-o", out, in}, &buf); rc != 0 {
		t.Fatalf("rc=%d %q", rc, buf.String())
	}
}

// --- usage errors (exit 2) ----------------------------------------------------

func TestObjcopy_UsageErrors(t *testing.T) {
	dir := t.TempDir()
	in := writeELF(t, dir, elf.EM_AARCH64)
	cases := [][]string{
		{"objcopy", in},                                           // missing -o
		{"objcopy", "-o", "x"},                                    // wrong arg count
		{"objcopy", "-O", "what", "-o", "x", in},                  // unknown format
		{"objcopy", "--uimage-arch", "z80", "-o", "x", in},        // bad uimage arch
		{"objcopy", "--load", "xx", "-O", "srec", "-o", "x", in},  // bad --load
		{"objcopy", "--entry", "xx", "-O", "srec", "-o", "x", in}, // bad --entry
		{"objcopy", "--no-such-flag"},                             // flag parse error
	}
	for _, args := range cases {
		var buf bytes.Buffer
		if rc := run(args, &buf); rc != 2 {
			t.Errorf("args %v: rc=%d, want 2 (%q)", args, rc, buf.String())
		}
	}
}

// --- runtime errors (exit 1) --------------------------------------------------

func TestObjcopy_RuntimeErrors(t *testing.T) {
	dir := t.TempDir()
	in := writeELF(t, dir, elf.EM_AARCH64)

	// missing input
	var b1 bytes.Buffer
	if rc := run([]string{"objcopy", "-o", filepath.Join(dir, "o"), filepath.Join(dir, "nope.elf")}, &b1); rc != 1 {
		t.Errorf("missing input rc=%d", rc)
	}
	// not an ELF
	bad := filepath.Join(dir, "bad.elf")
	writeFile(t, bad, []byte("not elf"))
	var b2 bytes.Buffer
	if rc := run([]string{"objcopy", "-o", filepath.Join(dir, "o"), bad}, &b2); rc != 1 {
		t.Errorf("bad elf rc=%d", rc)
	}
	// ELF with no loadable file content → Flatten error
	noload := filepath.Join(dir, "noload.elf")
	writeFile(t, noload, buildELF(t, elf.EM_AARCH64, 0, 0x1000, nil))
	var b3 bytes.Buffer
	if rc := run([]string{"objcopy", "-o", filepath.Join(dir, "o"), noload}, &b3); rc != 1 {
		t.Errorf("noload rc=%d", rc)
	}
	// output write error (nonexistent dir)
	var b4 bytes.Buffer
	if rc := run([]string{"objcopy", "-o", filepath.Join(dir, "no", "such", "o"), in}, &b4); rc != 1 {
		t.Errorf("write rc=%d", rc)
	}
}

// --- values that do not fit are refused, never truncated -----------------------

func TestObjcopy_RefusesWhatDoesNotFit(t *testing.T) {
	dir := t.TempDir()
	highBase := filepath.Join(dir, "highbase.elf")
	writeFile(t, highBase, buildELF(t, elf.EM_X86_64, 0x1000, 0xffffffff80000000, []byte("BAREMETAL")))
	highEntry := filepath.Join(dir, "highentry.elf")
	writeFile(t, highEntry, buildELF(t, elf.EM_X86_64, 0xffffffff80001000, 0x1000, []byte("BAREMETAL")))
	nearTop := filepath.Join(dir, "neartop.elf")
	writeFile(t, nearTop, buildELF(t, elf.EM_ARM, 0, 0xFFFFFFFE, []byte("BAREMETAL")))
	small := writeELF(t, dir, elf.EM_AARCH64)

	cases := []struct {
		name string
		args []string
		rc   int
		want string // in the output, for a refusal
	}{
		{"base above 32 bits, srec", []string{"-O", "srec", highBase}, 1, "image base"},
		{"base above 32 bits, ihex", []string{"-O", "ihex", highBase}, 1, "image base"},
		{"base above 32 bits, uimage", []string{"-O", "uimage", highBase}, 1, "image base"},
		{"base above 32 bits, binary needs no address", []string{"-O", "binary", highBase}, 0, ""},
		{"base above 32 bits with --load", []string{"-O", "ihex", "--load", "0x1000", highBase}, 0, ""},
		{"entry above 32 bits, srec", []string{"-O", "srec", highEntry}, 1, "ELF entry"},
		{"entry above 32 bits, uimage", []string{"-O", "uimage", highEntry}, 1, "ELF entry"},
		{"entry above 32 bits, ihex has no entry", []string{"-O", "ihex", highEntry}, 0, ""},
		{"entry above 32 bits with --entry", []string{"-O", "srec", "--entry", "0x1000", highEntry}, 0, ""},
		{"srec past 4 GiB", []string{"-O", "srec", nearTop}, 1, "4 GiB"},
		{"ihex past 4 GiB", []string{"-O", "ihex", nearTop}, 1, "4 GiB"},
		{"binary near the top is fine", []string{"-O", "binary", nearTop}, 0, ""},
		{"--max-size below the image", []string{"--max-size", "2", small}, 1, "larger than the allowed size"},
		{"--max-size at the image", []string{"--max-size", "9", small}, 0, ""},
		{"--max-size not a number", []string{"--max-size", "x", small}, 2, "--max-size"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "out")
			var buf bytes.Buffer
			rc := run(append([]string{"objcopy", "-o", out}, c.args...), &buf)
			if rc != c.rc {
				t.Fatalf("rc = %d, want %d (%q)", rc, c.rc, buf.String())
			}
			if c.rc != 0 && !bytes.Contains(buf.Bytes(), []byte(c.want)) {
				t.Errorf("output %q does not contain %q", buf.String(), c.want)
			}
			_, err := os.Stat(out)
			if (err == nil) != (c.rc == 0) {
				t.Errorf("output file exists = %v, want %v", err == nil, c.rc == 0)
			}
		})
	}
}

// --- internal helpers ---------------------------------------------------------

func TestUimageArchFromName(t *testing.T) {
	cases := map[string]byte{
		"": 0, "arm": fwimg.ArchARM, "arm64": fwimg.ArchARM64, "aarch64": fwimg.ArchARM64,
		"x86_64": fwimg.ArchX86_64, "amd64": fwimg.ArchX86_64, "riscv": fwimg.ArchRISCV,
		"riscv64": fwimg.ArchRISCV, "loongarch": fwimg.ArchLoongArch, "loong64": fwimg.ArchLoongArch,
	}
	for in, want := range cases {
		got, err := uimageArchFromName(in)
		if err != nil || got != want {
			t.Errorf("uimageArchFromName(%q) = %d,%v want %d", in, got, err, want)
		}
	}
	if _, err := uimageArchFromName("z80"); err == nil {
		t.Error("want error for z80")
	}
}

func TestUimageArchFromELF(t *testing.T) {
	cases := map[elf.Machine]byte{
		elf.EM_ARM: fwimg.ArchARM, elf.EM_AARCH64: fwimg.ArchARM64,
		elf.EM_X86_64: fwimg.ArchX86_64, elf.EM_RISCV: fwimg.ArchRISCV,
		elf.EM_LOONGARCH: fwimg.ArchLoongArch, elf.EM_MIPS: 0,
	}
	for m, want := range cases {
		if got := uimageArchFromELF(m); got != want {
			t.Errorf("uimageArchFromELF(%v) = %d, want %d", m, got, want)
		}
	}
}
