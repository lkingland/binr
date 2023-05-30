package binr_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/lkingland/binr"
)

// TestGet ensures the base case of downloading a binary on demand.
//
// For example, assuming "myapp" needs "testbin v1.0.0", the following call:
//
//	binr.Get("myapp", "testbin", "v1.0.0", source)
//
// (where "source" is a function which returns taht the binary can be found,
// example: http://binarySourceServer.example.com/v1.0.0/linux/amd64/testbin")
//
//	The returned path is the absolute path on disk where this binary can be
//	invoked.
func TestGet(t *testing.T) {

	ctx := context.Background()
	serverAddress := setupTestGet(t)

	// Get myapp/testbin-v1.0.0
	path, err := binr.Get(ctx, "myapp", "testbin", "v1.0.0",
		func(vers, os, arch string) (url, sum string, err error) {
			return fmt.Sprintf("http://%v/%v/%v/%v/testbin", serverAddress, vers, os, arch), "", nil
		})
	if err != nil {
		t.Fatal(err)
	}

	// Run it
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(path)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err = cmd.Run(); err != nil {
		t.Fatal(err)
	}

	// Validate its output
	if strings.TrimSpace(stdout.String()) != "OK" {
		t.Logf("stdout:\n%v", cmd.Stdout)
		t.Logf("stderr:\n%v", cmd.Stderr)
		t.Fatal("Command executed, but unexpected output received")
	}
}

// TODO: Several more tests are needed because the above only confirms the
// basic, happy path.
//
// TestGet_Checksum ensures that a checksum is checked if Source
// provides a URL to one.
// func TestGet_Checksum(t *testing.T) {}
//
// TestGet_Cache ensures that files are cached by calling Get for the same
// dependency twice, and ensuring that the handler is only invoked the once.
// func TestGet_Cache(t *testing.T) {}
//
// TestGet_Namespaced ensures that versions are correctly namespaced.  A
// version installed for one namespace does not update another.
//
// TestGet_NamespaceCache ensures that, when installing a previously installed
// command into a new namespace, the cache is still used.
//
// TestGet_Unversioned ensures that the binary can be invoked without its
// version suffix (the unversioned symlink was also created)
// func TestGet_Unversioned(t *testing.T) {}
//
// TestGet_Previous ensures that fetching an earlier version of
// a dependency does result in it being availble by explicit version path, but
// does not update the unversioned symlink.  It should stay always pointing at
// the latest.
// func TestGet_Previous(t *testing.T) {}
//

// TODO: A few additional features.
//
// TestGet_Latest ensures that explicitly providing "latest" will be passed
// through as an instruction to the source to fetch the latest version
// it can find.  (new feature)
// TestGet_Latest(t *testing.T)
//
// TestGet_Major ensures that requesting only the major version results
// in the latest release for that major version being isntalled
//
// TestGet_Minor ensures that requesting only the major version results
// in the latest release for that major version being isntalled
//
// TestUpdate ensures that requesting that a binary be updated causes the
// abolute latest version to be installed, as well as the latest for each
// of the major and minor versions installed.
// For example, if the versions of myapp released, were:
//   v2.0.0
//   v1.3.0
//   v1.2.2
//   v1.2.1
//   v1.2.0
//   v1.1.2
//   v1.1.1
//   v1.0.0
//
// Previous calls to .Get have resulted in the following versions being
// installed locally, and then the updated target after a later .Update:
//
//   Invocation       Symlink      Initial Target  Target afte .Update
//   Get "latest"  => mybin        -> v1.2.1       -> v2.0.0
//   Get "v1"      => mybin-v1     -> v1.2.1       -> v1.3.0
//   Get "v1.1"    => mybin-v1.1   -> v1.1.1       -> v1.1.2
//   Get "v1.1.2"  => mybin-v1.1.2 -> v1.1.2       -> v1.1.2 (unchanged)

// TestPath ensures that the expected absolute path is returned from the
// Path method.
func TestPath(t *testing.T) {
	// Set to homeless
	if runtime.GOOS != "linux" &&
		runtime.GOOS != "darwin" &&
		runtime.GOOS != "windows" {
		t.Logf("skipping")
		return
	}
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	// First check that we are indeed receiving an error that Home is not set
	if dir, err := os.UserHomeDir(); err == nil {
		t.Fatalf("did not receive expected os.UserHomDir error. got %q", dir)
	}

	// All tests presume the namespace "myapp" and command "mybin".  The
	// current working directory will be the default location for the binr
	// directory and cache when ther is no home nor XDG_CONFIG_HOME
	cwd, _ := os.Getwd()

	tests := []struct {
		name            string
		home, xdg, vers string
		expected        string
		err             bool
	}{
		{"homeless defaults",
			"", "", "",
			cwd + "/binr/myapp/mybin", false},
		{"home only",
			"/users/alice", "", "",
			"/users/alice/.config/binr/myapp/mybin", false},
		{"xdg only",
			"", "/users/alice/.xdg_config", "",
			"/users/alice/.xdg_config/binr/myapp/mybin", false},
		{"home and xdg",
			"/users/alice", "/users/alice/.xdg_config", "",
			"/users/alice/.xdg_config/binr/myapp/mybin", false},
		{"home with version",
			"/users/alice", "", "v1.0.0",
			"/users/alice/.config/binr/myapp/mybin-v1.0.0", false},
		{"invalid version",
			"/users/alice", "", "foo",
			"", true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("HOME", test.home)
			t.Setenv("USERPROFILE", test.home)
			t.Setenv("XDG_CONFIG_HOME", test.xdg)

			path, err := binr.Path("myapp", "mybin", test.vers)
			if err != nil && !test.err {
				t.Fatal(err) // errored and test not expected
			} else if err == nil && test.err {
				t.Fatal("did not receive expected error")
			}
			if path != test.expected {
				t.Fatalf("expected %q got %q", test.expected, path)
			}
		})
	}
}

// Helpers

func serveBinaries(t *testing.T) (string, error) {
	t.Helper()
	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
			return
		}
		filepath := filepath.Join("testbins", r.URL.Path) // TODO: Convert slashes for windows :(

		file, err := os.Open(filepath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
		defer file.Close()

		w.Header().Set("Content-Type", "application/octet-stream")

		_, err = io.Copy(w, file)
		if err != nil {
			log.Printf("Failed to write response: %v", err)
			http.Error(w, fmt.Sprintf("Failed to write response, %v", err), http.StatusInternalServerError)
		}

	})

	listener, err := net.Listen("tcp4", "127.0.0.1:")
	if err != nil {
		log.Fatalf("Failed to create listener: %v", err)
		return "", err
	}

	server := http.Server{Handler: handler}
	go func() {
		if err = server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "error serving: %v", err)
		}
	}()
	t.Cleanup(func() {
		_ = server.Close()
	})

	return listener.Addr().String(), nil

}

func setupTestGet(t *testing.T) (serverAddr string) {
	// Use a temp directory instead of ~/.config
	dir := t.TempDir()
	os.Setenv("XDG_CONFIG_HOME", dir)

	// Create a mock binaries source running on localhost.
	addr, err := serveBinaries(t)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}
