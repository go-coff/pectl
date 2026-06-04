package cmd

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// minimalPIE builds a tiny ET_DYN LoongArch PIE ELF with one R+W+X
// PT_LOAD segment at vaddr 0x10000 and a single R_LARCH_RELATIVE dynamic
// relocation — the minimal input linker.LinkPIE accepts.
func minimalPIE(t *testing.T) []byte {
	t.Helper()
	const (
		phoff      = 64
		segDataOff = 0x1000
		relaOff    = 0x2000
		shstrOff   = 0x2800
		shoff      = 0x3000
	)
	buf := make([]byte, 0x4000)

	copy(buf, []byte{0x7f, 'E', 'L', 'F'})
	buf[4], buf[5], buf[6] = 2, 1, 1 // ELFCLASS64, LSB, EV_CURRENT
	binary.LittleEndian.PutUint16(buf[16:], uint16(elf.ET_DYN))
	binary.LittleEndian.PutUint16(buf[18:], uint16(elf.EM_LOONGARCH))
	binary.LittleEndian.PutUint32(buf[20:], 1)
	binary.LittleEndian.PutUint64(buf[24:], 0x10000) // e_entry
	binary.LittleEndian.PutUint64(buf[32:], phoff)
	binary.LittleEndian.PutUint64(buf[40:], shoff)
	binary.LittleEndian.PutUint16(buf[52:], 64)
	binary.LittleEndian.PutUint16(buf[54:], 56)
	binary.LittleEndian.PutUint16(buf[56:], 1) // e_phnum
	binary.LittleEndian.PutUint16(buf[58:], 64)

	// PT_LOAD R+W+X
	binary.LittleEndian.PutUint32(buf[phoff:], uint32(elf.PT_LOAD))
	binary.LittleEndian.PutUint32(buf[phoff+4:], uint32(elf.PF_R|elf.PF_W|elf.PF_X))
	binary.LittleEndian.PutUint64(buf[phoff+8:], segDataOff)
	binary.LittleEndian.PutUint64(buf[phoff+16:], 0x10000)
	binary.LittleEndian.PutUint64(buf[phoff+24:], 0x10000)
	binary.LittleEndian.PutUint64(buf[phoff+32:], 0x100)
	binary.LittleEndian.PutUint64(buf[phoff+40:], 0x200)
	binary.LittleEndian.PutUint64(buf[phoff+48:], 0x1000)

	// one R_LARCH_RELATIVE at vaddr 0x10010
	binary.LittleEndian.PutUint64(buf[relaOff:], 0x10010)
	binary.LittleEndian.PutUint64(buf[relaOff+8:], uint64(elf.R_LARCH_RELATIVE))
	binary.LittleEndian.PutUint64(buf[relaOff+16:], 0x10040)

	names := "\x00.text\x00.rela\x00.shstrtab\x00"
	copy(buf[shstrOff:], names)

	type sh struct {
		name             uint32
		typ              elf.SectionType
		flags            uint64
		off, size        uint64
		entsz, addralign uint64
	}
	shs := []sh{
		{},
		{name: uint32(strings.Index(names, ".text")), typ: elf.SHT_PROGBITS, flags: uint64(elf.SHF_ALLOC | elf.SHF_EXECINSTR), off: segDataOff, size: 0x100, addralign: 16},
		{name: uint32(strings.Index(names, ".rela")), typ: elf.SHT_RELA, off: relaOff, size: 24, entsz: 24, addralign: 8},
		{name: uint32(strings.Index(names, ".shstrtab")), typ: elf.SHT_STRTAB, off: shstrOff, size: uint64(len(names)), addralign: 1},
	}
	binary.LittleEndian.PutUint16(buf[60:], uint16(len(shs)))
	binary.LittleEndian.PutUint16(buf[62:], uint16(len(shs)-1))
	for i, x := range shs {
		o := shoff + i*64
		binary.LittleEndian.PutUint32(buf[o:], x.name)
		binary.LittleEndian.PutUint32(buf[o+4:], uint32(x.typ))
		binary.LittleEndian.PutUint64(buf[o+8:], x.flags)
		binary.LittleEndian.PutUint64(buf[o+24:], x.off)
		binary.LittleEndian.PutUint64(buf[o+32:], x.size)
		binary.LittleEndian.PutUint64(buf[o+48:], x.addralign)
		binary.LittleEndian.PutUint64(buf[o+56:], x.entsz)
	}
	return buf
}

// --- happy path ---------------------------------------------------------------

func TestLinkPIE_Convert(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "hello-pie.elf")
	out := filepath.Join(dir, "out.efi")
	writeFile(t, in, minimalPIE(t))

	var buf bytes.Buffer
	rc := run([]string{"link-pie", "-o", out, in}, &buf)
	if rc != 0 {
		t.Fatalf("rc=%d stderr=%q", rc, buf.String())
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(raw, []byte("MZ")) {
		t.Error("output is not a PE")
	}
}

func TestLinkPIE_AllFlags(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "p.elf")
	out := filepath.Join(dir, "out.efi")
	writeFile(t, in, minimalPIE(t))
	var buf bytes.Buffer
	rc := run([]string{
		"link-pie",
		"--subsystem", "10",
		"--image-base", "0x10000",
		"--section-alignment", "4096",
		"--file-alignment", "512",
		"-o", out, in,
	}, &buf)
	if rc != 0 {
		t.Fatalf("rc=%d stderr=%q", rc, buf.String())
	}
}

// --- usage errors (exit 2) ----------------------------------------------------

func TestLinkPIE_UsageMissingOutput(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "p.elf")
	writeFile(t, in, minimalPIE(t))
	var buf bytes.Buffer
	rc := run([]string{"link-pie", in}, &buf)
	if rc != 2 {
		t.Fatalf("rc=%d, want 2", rc)
	}
	if !strings.Contains(buf.String(), "output") {
		t.Errorf("stderr should mention --output, got %q", buf.String())
	}
}

func TestLinkPIE_UsageWrongArgCount(t *testing.T) {
	var buf bytes.Buffer
	if rc := run([]string{"link-pie", "-o", "out.efi"}, &buf); rc != 2 {
		t.Fatalf("rc=%d, want 2", rc)
	}
}

func TestLinkPIE_UsageBadImageBase(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "p.elf")
	writeFile(t, in, minimalPIE(t))
	var buf bytes.Buffer
	rc := run([]string{"link-pie", "--image-base", "xyz", "-o", "out.efi", in}, &buf)
	if rc != 2 {
		t.Fatalf("rc=%d, want 2", rc)
	}
}

func TestLinkPIE_UsageFlagParseError(t *testing.T) {
	var buf bytes.Buffer
	if rc := run([]string{"link-pie", "--no-such-flag"}, &buf); rc != 2 {
		t.Fatalf("rc=%d, want 2", rc)
	}
}

// --- runtime errors (exit 1) --------------------------------------------------

func TestLinkPIE_InputReadError(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.efi")
	var buf bytes.Buffer
	rc := run([]string{"link-pie", "-o", out, filepath.Join(dir, "missing.elf")}, &buf)
	if rc != 1 {
		t.Fatalf("rc=%d, want 1", rc)
	}
}

func TestLinkPIE_InputNotPIE(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "garbage.elf")
	out := filepath.Join(dir, "out.efi")
	writeFile(t, bad, []byte("not an elf at all"))
	var buf bytes.Buffer
	rc := run([]string{"link-pie", "-o", out, bad}, &buf)
	if rc != 1 {
		t.Fatalf("rc=%d, want 1", rc)
	}
}

func TestLinkPIE_OutputWriteError(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "p.elf")
	writeFile(t, in, minimalPIE(t))
	out := filepath.Join(dir, "no", "such", "dir", "out.efi")
	var buf bytes.Buffer
	rc := run([]string{"link-pie", "-o", out, in}, &buf)
	if rc != 1 {
		t.Fatalf("rc=%d, want 1", rc)
	}
}
