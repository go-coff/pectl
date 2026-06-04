package cmd

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"strings"
	"testing"
)

// --- shared helpers (used by append_test.go and sign_test.go) ---------------

// buildMinimalPE returns a tiny but well-formed PE32+ image that pe.Append
// can extend.
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

// --- root-only tests ---------------------------------------------------------

func TestUsageErr_Error(t *testing.T) {
	if got := (usageErr{"hello"}).Error(); got != "hello" {
		t.Errorf("usageErr.Error() = %q, want %q", got, "hello")
	}
}

// TestRun_BareHelp: invoking `pectl` with no args is not an error; cobra
// prints the root help and exits 0. Other CLI tools (git, docker, kubectl)
// behave the same way.
func TestRun_BareHelp(t *testing.T) {
	var buf bytes.Buffer
	if rc := run(nil, &buf); rc != 0 {
		t.Fatalf("rc=%d, want 0 (bare pectl shows help)", rc)
	}
	if !strings.Contains(buf.String(), "Available Commands") {
		t.Errorf("stderr should list subcommands, got %q", buf.String())
	}
}

// TestRun_RootFlagError: an unknown flag at root level is a usage error.
func TestRun_RootFlagError(t *testing.T) {
	var buf bytes.Buffer
	if rc := run([]string{"--unknown-flag"}, &buf); rc != 2 {
		t.Fatalf("rc=%d, want 2", rc)
	}
}

// TestRun_UnknownCommand: an unknown subcommand is a runtime error in cobra's
// model (it doesn't go through SetFlagErrorFunc). We accept exit code 1
// rather than over-engineering an interceptor.
func TestRun_UnknownCommand(t *testing.T) {
	var buf bytes.Buffer
	if rc := run([]string{"nosuchcommand"}, &buf); rc != 1 {
		t.Fatalf("rc=%d, want 1", rc)
	}
}

// TestExecute drives the canonical entry point through intercepted os.Args
// and os.Stderr, since Execute() reads them directly.
func TestExecute(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"pectl"} // bare → help → exit 0

	saved := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	defer func() {
		os.Stderr = saved
		w.Close()
		_, _ = io.Copy(io.Discard, r)
	}()

	if rc := Execute(); rc != 0 {
		t.Fatalf("Execute() = %d, want 0", rc)
	}
}
