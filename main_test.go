package main

import (
	"io"
	"os"
	"testing"
)

// TestMain_Entry drives main() with osExit and os.Args intercepted, so its
// single statement is covered without actually terminating the test process.
// The cmd package's own tests cover Execute() and everything below it.
func TestMain_Entry(t *testing.T) {
	oldExit, oldArgs := osExit, os.Args
	defer func() { osExit = oldExit; os.Args = oldArgs }()

	var gotCode int
	osExit = func(c int) { gotCode = c; panic("exit") }
	// `pectl append` with no positional → ExactArgs(1) fails → exit 2.
	// (`pectl` bare prints help and would exit 0, which wouldn't exercise
	// the non-zero osExit path.)
	os.Args = []string{"pectl", "append"}

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
