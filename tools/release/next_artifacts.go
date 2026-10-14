package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"debug/macho"
	"debug/pe"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

type nextArtifact struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type nextInventory struct {
	Schema       string         `json:"schema"`
	Mode         string         `json:"mode"`
	ReleaseReady bool           `json:"release_ready"`
	Candidate    nextManifest   `json:"candidate"`
	Platform     string         `json:"platform"`
	Artifacts    []nextArtifact `json:"available_artifacts"`
}

// Only explicitly selected, canonical Next names are inventoried. This is not
// an installer completeness check, signature check or payload provenance proof.
func nextArtifactType(platform, name string) string {
	switch platform {
	case "darwin/arm64":
		switch name {
		case "AgentDock-Next-macos-arm64.zip":
			return "zip"
		case "AgentDock-Next-macos-arm64.dmg":
			return "dmg"
		case "agentdock-next-core", "agentdock-next-arbiter":
			return "mach-o"
		}
	case "windows/amd64", "windows/arm64":
		if name == "agentdock-next-core.exe" || name == "agentdock-next-arbiter.exe" {
			return "pe"
		}
		if name == "AgentDock-NextSetup-"+platform[len("windows/"):]+".exe" {
			return "pe"
		}
	}
	return ""
}

func checkNextArtifactType(f *os.File, size int64, kind, platform string) error {
	switch kind {
	case "zip":
		_, err := zip.NewReader(f, size)
		return err
	case "dmg":
		trailer := make([]byte, 512)
		if size < 512 {
			return errors.New("missing DMG trailer")
		}
		if _, err := f.ReadAt(trailer, size-512); err != nil {
			return err
		}
		if !bytes.Equal(trailer[:4], []byte("koly")) {
			return errors.New("missing UDIF signature")
		}
	case "mach-o":
		binary, err := macho.NewFile(f)
		if err != nil {
			return err
		}
		if binary.Cpu != macho.CpuArm64 || binary.Type != macho.TypeExec {
			return errors.New("expected arm64 Mach-O executable")
		}
	case "pe":
		binary, err := pe.NewFile(f)
		if err != nil {
			return err
		}
		machine := uint16(pe.IMAGE_FILE_MACHINE_AMD64)
		if platform == "windows/arm64" {
			machine = pe.IMAGE_FILE_MACHINE_ARM64
		}
		if binary.Machine != machine || binary.Characteristics&pe.IMAGE_FILE_EXECUTABLE_IMAGE == 0 || binary.Characteristics&pe.IMAGE_FILE_DLL != 0 {
			return errors.New("expected target PE executable")
		}
	default:
		return errors.New("unknown artifact type")
	}
	return nil
}

func inventoryNextArtifacts(m nextManifest, root, platform string, names []string) ([]byte, error) {
	if m.ReleaseReady {
		return nil, errors.New("source inventory cannot grant release readiness")
	}
	if len(names) == 0 {
		return nil, errors.New("select at least one available artifact")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}
	if resolved != abs {
		return nil, errors.New("artifact root must be canonical and non-symlink")
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return nil, errors.New("artifact root must be a directory")
	}
	names = append([]string(nil), names...)
	sort.Strings(names)
	result := nextInventory{"agentdock-next-artifacts-v1", "source-only-available-files", false, m, platform, nil}
	for i, name := range names {
		kind := nextArtifactType(platform, name)
		if kind == "" || filepath.Base(name) != name || (i > 0 && names[i-1] == name) {
			return nil, fmt.Errorf("noncanonical or duplicate Next artifact: %q", name)
		}
		path := filepath.Join(abs, name)
		before, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if !before.Mode().IsRegular() || before.Size() == 0 {
			return nil, errors.New("artifact must be a nonempty regular file, no symlinks")
		}
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		record, err := hashNextArtifact(f, before, name, kind, platform)
		closeErr := f.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
		after, err := os.Lstat(path)
		if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
			return nil, errors.New("artifact changed during inventory")
		}
		result.Artifacts = append(result.Artifacts, record)
	}
	data, err := json.MarshalIndent(result, "", "  ")
	return append(data, '\n'), err
}

func hashNextArtifact(f *os.File, before os.FileInfo, name, kind, platform string) (nextArtifact, error) {
	var record nextArtifact
	opened, err := f.Stat()
	if err != nil {
		return record, err
	}
	if !os.SameFile(before, opened) {
		return record, errors.New("artifact replaced during open")
	}
	if err := checkNextArtifactType(f, before.Size(), kind, platform); err != nil {
		return record, fmt.Errorf("%s: %w", name, err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return record, err
	}
	hash := sha256.New()
	size, err := io.Copy(hash, f)
	if err != nil {
		return record, err
	}
	if size != before.Size() {
		return record, errors.New("artifact size changed")
	}
	return nextArtifact{name, kind, size, hex.EncodeToString(hash.Sum(nil))}, nil
}

func runNextArtifacts(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("next-artifacts", flag.ContinueOnError)
	manifest := fs.String("manifest", "", "canonical clean-HEAD source manifest")
	root := fs.String("root", "", "canonical directory containing available artifacts")
	platform := fs.String("platform", "", "darwin/arm64 or windows/amd64 or windows/arm64")
	verify := fs.String("verify", "", "exact canonical inventory to compare")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *manifest == "" || *root == "" {
		return errors.New("next-artifacts requires --manifest, --root, --platform and filenames")
	}
	data, err := os.ReadFile(*manifest)
	if err != nil {
		return err
	}
	// Reuse the existing clean-HEAD, date, identity and exact manifest gate.
	if err := runNextCandidate([]string{"--manifest", *manifest}, io.Discard); err != nil {
		return err
	}
	checked, err := os.ReadFile(*manifest)
	if err != nil {
		return err
	}
	if !bytes.Equal(data, checked) {
		return errors.New("manifest changed during validation")
	}
	var m nextManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	encoded, err := inventoryNextArtifacts(m, *root, *platform, fs.Args())
	if err != nil {
		return err
	}
	if *verify != "" {
		expected, err := os.ReadFile(*verify)
		if err != nil {
			return err
		}
		if err := verifyNextInventory(encoded, expected); err != nil {
			return err
		}
	}
	_, err = stdout.Write(encoded)
	return err
}

func verifyNextInventory(actual, expected []byte) error {
	if !bytes.Equal(actual, expected) {
		return errors.New("Next inventory mismatch (canonical JSON required)")
	}
	return nil
}
