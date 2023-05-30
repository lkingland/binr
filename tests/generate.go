package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// This file generates a path on disk suitable for use by the test
// binaries server which hosts a binary on localhost
// at the path v1.0.0/{OS}/{ARCH}/testbin
// This is involed by "make" as a prerequisite to the "test" target.
func main() {

	var (
		vers = "v1.0.0" // For now all tests are for v1.0.0
		path = fmt.Sprintf("testbins/%s/%s/%s/testbin", vers, runtime.GOOS, runtime.GOARCH)
	)

	// Generates test binaries for the current system
	cmd := exec.Command("go")
	cmd.Args = []string{"go", "build", "-o", path, "./tests/testbin"}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
