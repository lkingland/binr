// package binr provides facilities for downloading and interacting
// with binaries on the local system programmatically.

package binr

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Masterminds/semver"
	"github.com/rs/zerolog/log"
)

// DefaultLogLevel for binr is logging disabled.
// Use SetLogLevel to change.
const DefaultLogLevel = LogDisabled

// Get the path to a binary.
//
// `namespace` is generally the name of the application or system
// which is utilizing binr.  By default commands will be downloaded
// to this namespace within the current user's config directory
// (~/.config/binr/[namespace])
//
// `command` is the name of the command to get.  It is made available by
// default as ~/.config/binr/[namespace]/[command]
// and also   ~/.config/binr/[namespace]/[command]-[version]
//
// Version is the specific version to get. TODO: currently expects an exact
// (vX.Y.Z), but will soon support semver major and minor (vX and vX.Y).
//
// The provided Source is a function which returns a final location at
// which the command and its checksum can be downloaded for a given os,
// architecture and version.
func Get(ctx context.Context, namespace, command, version string, source Source, options ...option) (path string, err error) {
	cfg := newConfig(options...)

	log.Debug().
		Str("namespace", namespace).
		Str("command", command).
		Str("version", version).
		Bool("update", cfg.update).
		Msg("binr ensuring command")

	if namespace == "" {
		return "", errors.New("binr Get requires namespace")
	} else if command == "" {
		return "", errors.New("binr Get requires command")
	} else if version == "" {
		return "", errors.New("binr Get requires a version")
	} else if _, err := semver.NewVersion(version); err != nil {
		return "", errors.New("binr Get requires version to be a valid semver (ex: v1.2.3)")
	} else if source == nil {
		return "", errors.New("binr Get requires a Source to resolve missing dependencies")
	} else if cfg.update {
		return "", errors.New("binr Get WithUpdate is not yet implemented")
	}

	if err = setup(); err != nil {
		return
	}

	if path, err = Path(namespace, command, version); err != nil {
		return
	}

	if got(path) {
		log.Debug().Str("path", path).Msg("binr found command locally")
		return
	}

	sourceURL, sumURL, err := source(version, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return
	}

	sum, err := getChecksum(ctx, sumURL) // URL to checksum (optional)
	if err != nil {
		return
	}

	sum, cleanup, err := cache(ctx, sourceURL, sum) // returns actual sum if no sumURL provided
	if err != nil {
		return
	}
	defer cleanup()

	if err = link(namespace, command, version, sum); err != nil {
		return
	}
	log.Debug().Msg("binr completed without error")
	return
}

// Source is a function which, when provided a version, OS and architecture
// will return the urls at which the binary and its checksum can be found.
type Source func(version, os, arch string) (url, sum string, err error)

// config is mutated by functional options for Get such as WithUpdate
type config struct{ update bool }

type option func(*config)

func newConfig(options ...option) (cfg config) {
	for _, option := range options {
		option(&cfg)
	}
	return
}

// WithUpdate instructs the system to update extant binaries.
// The default behavior is to never replace a binary once it has been provided.
// Note that this option worls in concert with version and checksum URL.
// TODO: implement and document:
//
//	Has no effect if either the version is explicit (vX.Y.Z), or there is
//	no checksum URL returned from the source implementation.
//	If the version is omitted (aka "latest"), but a checksum URL is provided
//	from the source implementation, WithUpdate will result in a call to
//	.Get checking if the checksum differs from the currently cached binary,
//	and replacing with the latest via download.
//	If the version is provided, but is not explicit (vX.Y or vX), the
//	source implementation should provide the checksum URL and source URL to
//	the latest implementation which adheres to the specified semver, and
//	the WithUpdate option causes this to be utilized.
func WithUpdate() func(*config) {
	return func(c *config) { c.update = true }
}

// setup ensures that the binr cache directory is available
func setup() (err error) {
	path := cachePath()
	if _, err = os.Stat(path); os.IsNotExist(err) {
		log.Debug().Str("path", path).Msg("creating local binr cache")
		if err = os.MkdirAll(path, os.ModePerm); err != nil {
			return fmt.Errorf("binr was unable to create cache directory. %w", err)
		}
	}
	if err != nil {
		return fmt.Errorf("binr encountered an unexpected error accessing its cache. %w", err)
	}
	return
}

// cachePath returns the effective path to the binr cache.
// In the event that there is neither a home directory nor an XDG_CONFIG_HOME
// set, the relative path ".binr/bin" is used.
func cachePath() (path string) {
	path, _ = filepath.Abs(filepath.Join(dotfilesPath(), "binr", ".cache"))
	return
}

// Path returns the absolute path at which the given command for
// the given namespace is expected to exist.  It does not validate the
// command's existence (see Get).
//
// Version is optional, and if not provided will point to a "floating"
// link which is always updated to the current version.  If provided, it
// must be a semver
func Path(namespace, command, version string) (path string, err error) {
	if namespace == "" {
		return "", errors.New("binr Path requires namespace")
	} else if command == "" {
		return "", errors.New("binr Path requires command")
	} else if version != "" {
		if _, err := semver.NewVersion(version); err != nil {
			return "", errors.New("binr Path requires version to be a valid semver (ex: v1.2.3)")
		}
		command += "-" + version
	}
	return filepath.Abs(filepath.Join(dotfilesPath(), "binr", namespace, command))
}

// dotfilesPath returns ~/.config by default, XDG_CONFIG_HOME if set, or
// In the event that there is neither a home directory nor an XDG_CONFIG_HOME
// set, the relative path ".binr/bin" is used.
func dotfilesPath() string {
	var (
		xdg           = os.Getenv("XDG_CONFIG_HOME")
		home, homeErr = os.UserHomeDir()
		dotfiles      = filepath.Join(home, ".config")
	)
	if homeErr != nil && xdg == "" {
		log.Warn().Msg("binr found no home directory nor XDG_CONFIG_HOME environment variable.  The current working directory will be used.")
		return "."
	}
	if xdg != "" {
		dotfiles = xdg
	}
	return dotfiles
}

// got the command already?
func got(path string) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}
	// TODO: ensure it is a symlink
	// TODO: ensure it points to a path in the store
	// TODO: ensure target exists
	// TODO: ensure target is executable
	// TODO: ensure target filename is its hash
	return true
}

// getChecksum returns the checksum at the given URL if provided, empty string
// otherwise.  If provided, any error turning the URL into a checksum is
// bubbled.
func getChecksum(ctx context.Context, url string) (string, error) {
	if url == "" {
		return "", nil
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("binr was unable to fetch the command's checksum from url %q. %w", url, err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", fmt.Errorf("binr received an HTTP %v from checksum URL %q", res.StatusCode, url)
	}
	bb, err := io.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("binr received an error reading the checksum URL %q. %w", url, err)
	}
	return strings.TrimSpace(string(bb)), nil
	// TODO: confirm the format of the body appears to be a checksum
}

// cache the binary at the given URL which should have the given checksum.
// If a command already exists in the storw with the given checksum, it is
// already cached and a fetch is not initiated.
// The checksum is optional, used to check for cached copies and validate
// download integrity if provided.
// NOTE: future versions will consider the semver and staleness.
func cache(ctx context.Context, url, checksum string) (sum string, done func(), err error) {
	log.Debug().
		Str("url", url).
		Str("checksum", checksum).
		Msg("binr sourcing command")

	if cached(checksum) {
		return
	}

	t := time.Now()
	tmpfile := filepath.Join(cachePath(), fmt.Sprint(t.Format("20060102150405.999"))+".partial")

	done = func() {
		log.Debug().Msg("binr cleaning up")
		// TODO: in the event of a panic this deferred cleanup will not fire.
		// This could be rearchitected by, for example, using a guid encoded
		// in the partial filename and and PID.  Finalization then uses only the
		// partial with the current GUID, and upon success removes all partials
		// whose encoded pid is no longer a running process.  This cleanup could
		// be run as an initial task in setup.
		if _, err := os.Stat(tmpfile); os.IsNotExist(err) {
			return
		}
		if err := os.Remove(tmpfile); err != nil {
			log.Warn().Err(err).Msg("binr unable to remove partial download.")
		}
	}

	if err = download(ctx, url, tmpfile, "application/octet-stream"); err != nil {
		return
	}

	if checksum == "" {
		if checksum, err = calculateChecksum(tmpfile); err != nil {
			return
		}
	} else {
		if err = verify(tmpfile, checksum); err != nil {
			return
		}
	}

	newpath := filepath.Join(cachePath(), checksum)
	log.Debug().
		Str("from", tmpfile).
		Str("to", newpath).
		Msg("moving into place")

	return checksum, done, os.Rename(tmpfile, newpath)
}

// download the given url to the given output, (optionally) verifying the
// content type
func download(ctx context.Context, url, outPath, contentType string) error {
	if _, err := os.Stat(outPath); err == nil {
		return fmt.Errorf("binr encountered an existing download file. If you are sure it is from a failed earlier attempt, the file can be removed. %v", outPath)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("binr received an http error fetching the command. %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("binr received an HTTP %v from source URL %q", res.StatusCode, url)
	}
	if res.Header.Get("Content-Type") != contentType {
		return fmt.Errorf("binr unable to source command.  Source URL reported a content type of %q when an %q was expected", res.Header.Get("Content-Type"), contentType)
	}
	file, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("binr unable to open local file for writing. %w", err)
	}
	defer file.Close()
	if _, err = io.Copy(file, res.Body); err != nil {
		return fmt.Errorf("binr encoutered an error copying remote data. %w", err)
	}
	log.Debug().Str("path", outPath).Msg("binr download complete")
	return nil
}

// cached returns whether or not the binary with the given checksum exists
// in the cache.
func cached(checksum string) bool {
	if checksum == "" {
		return false
	}
	path := filepath.Join(cachePath(), checksum)
	_, err := os.Stat(path)
	return (err == nil)
}

// verify the given path has the given checksum
func verify(path, checksum string) (err error) {
	fileChecksum, err := calculateChecksum(path)
	if err != nil {
		return
	}
	if fileChecksum != checksum {
		log.Debug().
			Str("path", path).
			Str("expected", checksum).
			Str("calculated", fileChecksum).
			Msg("checksum mismatch")
		return errors.New("binr detected a checksum mismatch. Not sourcing command")
	}
	return
}

// calculateChecksum of file at path.
func calculateChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("binr unable to calculate file's checksum. %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("binr unable to calculate file's checksum. %w", err)
	}

	// hashInBytes := hash.Sum(nil)[:32]
	hashInBytes := hash.Sum(nil)
	return hex.EncodeToString(hashInBytes), nil
}

// link a new command to the cached object with the given checksum
func link(namespace, command, version, sum string) (err error) {
	pathVersioned, err := Path(namespace, command, version)
	if err != nil {
		return
	}
	target := filepath.Join("..", ".cache", sum)
	log.Debug().
		Str("target", target).
		Str("path", pathVersioned).
		Msg("linking versioned")

	if err = os.Mkdir(filepath.Dir(pathVersioned), os.ModePerm); err != nil {
		return
	}
	if err = os.Symlink(target, pathVersioned); err != nil {
		return
	}

	if ok, err := isNewer(namespace, command, version); !ok || err != nil {
		log.Debug().Msg("version linked is not newest. leaving unversioned link unchanged.")
		return err
	}

	pathUnversioned, err := Path(namespace, command, "")
	if err != nil {
		return
	}

	log.Debug().
		Str("target", target).
		Str("path", pathUnversioned).
		Msg("updating unversioned link")

	return os.Symlink(target, pathUnversioned)
}

// isNewer returns true if the given version would become the latest
// installed version of the command in the given namespace.
func isNewer(namespace, command, versionStr string) (bool, error) {
	dir := filepath.Join(dotfilesPath(), "binr", namespace)

	version, err := semver.NewVersion(versionStr)
	if err != nil {
		return false, fmt.Errorf("binr can not determine if the given command is the latest because an invalid semver was received: %q", versionStr)
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		return false, fmt.Errorf("binr unable to check for latest version. %w", err)
	}

	var highest *semver.Version

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		prefix := command + "-"
		if !strings.HasPrefix(file.Name(), prefix) {
			continue // other command or the unversioned (latest) link of this one
		}
		suffix := strings.TrimPrefix(file.Name(), prefix)

		v, err := semver.NewVersion(suffix)
		if err != nil {
			return false, fmt.Errorf("binr found a file which is not of the expected form [command]-[version]: %q. the version does not appear to be a Semver (v1.2.3)", file.Name())
		}
		if highest == nil || v.GreaterThan(highest) {
			highest = v
		}
	}

	return !highest.GreaterThan(version), nil
}
