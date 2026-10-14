package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Reuse the source contract and exact-byte verification; no imported JSON field
// gains authority before its identity, HEAD and timestamp have been checked.
func verifyNextCandidate(data []byte, root, head, date, dirty string) (nextManifest, error) {
	var supplied nextManifest
	if err := json.Unmarshal(data, &supplied); err != nil {
		return supplied, err
	}
	in := nextInput{supplied.Version, supplied.Tag, supplied.Commit, supplied.BuildDate, strings.TrimSuffix(supplied.MacBuildVersion, ".0.0")}
	m, err := nextContract(in)
	if err != nil {
		return m, err
	}
	canonical, err := json.MarshalIndent(m, "", "  ")
	if err != nil || !bytes.Equal(data, append(canonical, '\n')) {
		return m, errors.New("Next manifest mismatch (canonical JSON required)")
	}
	if dirty != "" || m.Commit != head || m.BuildDate != date {
		return m, errors.New("RC requires clean checkout and exact HEAD commit/build-date")
	}
	identity, err := os.ReadFile(filepath.Join(root, "packaging/macos/app-identity.sh"))
	if err != nil {
		return m, err
	}
	return m, verifyNextIdentity(identity)
}

func writeNextOverlay(root, output string, m nextManifest) error {
	source := filepath.Join(root, "internal/buildinfo/buildinfo.go")
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	stable := []byte(`const Version = "0.9.1"`)
	if bytes.Count(data, stable) != 1 {
		return errors.New("Stable buildinfo declaration changed; review Next overlay")
	}
	data = bytes.Replace(data, stable, []byte("const Version = "+strconv.Quote(m.Version)), 1)
	replacement := filepath.Join(output, "next-buildinfo.go")
	overlay, err := json.Marshal(map[string]any{"Replace": map[string]string{source: replacement}})
	if err != nil {
		return err
	}
	for _, file := range []struct {
		path string
		data []byte
	}{{replacement, data}, {filepath.Join(output, "overlay.json"), overlay}} {
		f, err := os.OpenFile(file.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, err = f.Write(file.data)
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func runNextCandidate(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("next-candidate", flag.ContinueOnError)
	manifest := fs.String("manifest", "", "canonical next-preflight manifest")
	output := fs.String("output", "", "existing repo-local dist directory for fresh overlay files")
	checkOutput := fs.String("check-output", "", "validate repo-local destination without writing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || *manifest == "" {
		return errors.New("next-candidate requires --manifest")
	}
	read := func(args ...string) (string, error) {
		out, err := exec.Command("git", args...).Output()
		return strings.TrimSpace(string(out)), err
	}
	root, err := read("rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	head, err := read("rev-parse", "HEAD")
	if err != nil {
		return err
	}
	stamp, err := read("show", "-s", "--format=%ct", "HEAD")
	if err != nil {
		return err
	}
	seconds, err := strconv.ParseInt(stamp, 10, 64)
	if err != nil {
		return err
	}
	dirty, err := read("status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return err
	}
	data, err := os.ReadFile(*manifest)
	if err != nil {
		return err
	}
	m, err := verifyNextCandidate(data, root, head, time.Unix(seconds, 0).UTC().Format(time.RFC3339), dirty)
	if err != nil {
		return err
	}
	for _, destination := range []string{*output, *checkOutput} {
		if destination == "" {
			continue
		}
		// Resolve existing paths before writing, rejecting symlinks and destinations
		// outside the ignored build directory (including live application paths).
		abs, err := filepath.Abs(destination)
		if err != nil {
			return err
		}
		parent := abs
		var suffix []string
		for {
			_, err := os.Lstat(parent)
			if err == nil {
				break
			}
			if !os.IsNotExist(err) {
				return err
			}
			suffix = append(suffix, filepath.Base(parent))
			parent = filepath.Dir(parent)
		}
		resolved, err := filepath.EvalSymlinks(parent)
		if err != nil {
			return err
		}
		for i := len(suffix) - 1; i >= 0; i-- {
			resolved = filepath.Join(resolved, suffix[i])
		}
		canonicalRoot, err := filepath.EvalSymlinks(root)
		if err != nil {
			return err
		}
		if resolved != abs || !strings.HasPrefix(resolved, filepath.Join(canonicalRoot, "dist")+string(os.PathSeparator)) {
			return errors.New("overlay output must be a non-symlink repo-local dist subdirectory")
		}
		if destination == *output {
			if err := writeNextOverlay(root, resolved, m); err != nil {
				return err
			}
		}
	}
	_, err = fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%s\n", m.Version, m.Commit, m.BuildDate, m.MacShortVersion, m.MacBuildVersion)
	return err
}
