package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNextArtifactsAvailableFiles(t *testing.T) {
	root, m, _ := candidateFixture(t)
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	entry, err := writer.Create("fixture.txt")
	if err != nil {
		t.Fatal(err)
	}
	entry.Write([]byte("fixture, not a packaged app"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	name := "AgentDock-Next-macos-arm64.zip"
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, buf.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	encoded, err := inventoryNextArtifacts(m, root, "darwin/arm64", []string{name})
	if err != nil {
		t.Fatal(err)
	}
	var got nextInventory
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(buf.Bytes())
	record := got.Artifacts[0]
	if got.ReleaseReady || got.Candidate.ReleaseReady || got.Candidate.Commit != m.Commit || got.Candidate.BuildDate != m.BuildDate || got.Candidate.Version != m.Version || len(got.Artifacts) != 1 || record.Type != "zip" || record.Size != int64(buf.Len()) || record.SHA256 != hex.EncodeToString(digest[:]) {
		t.Fatalf("wrong inventory: %s", encoded)
	}
	again, err := inventoryNextArtifacts(m, root, "darwin/arm64", []string{name})
	if err != nil || !bytes.Equal(encoded, again) {
		t.Fatal("nondeterministic inventory")
	}
	if err := verifyNextInventory(again, encoded); err != nil {
		t.Fatal(err)
	}
	for label, altered := range map[string][]byte{
		"hash":          bytes.Replace(encoded, []byte(record.SHA256), []byte(strings.Repeat("0", 64)), 1),
		"size":          bytes.Replace(encoded, []byte(`"size": `), []byte(`"size": 9`), 1),
		"type":          bytes.Replace(encoded, []byte(`"type": "zip"`), []byte(`"type": "dmg"`), 1),
		"commit":        bytes.ReplaceAll(encoded, []byte(m.Commit), []byte(strings.Repeat("b", 40))),
		"version":       bytes.ReplaceAll(encoded, []byte(m.Version), []byte("0.9.1-rc.3")),
		"date":          bytes.ReplaceAll(encoded, []byte(m.BuildDate), []byte("2000-01-01T00:00:00Z")),
		"ready":         bytes.ReplaceAll(encoded, []byte(`"release_ready": false`), []byte(`"release_ready": true`)),
		"unknown":       bytes.Replace(encoded, []byte("{\n"), []byte("{\n  \"unknown\": true,\n"), 1),
		"duplicate-key": bytes.Replace(encoded, []byte("{\n"), []byte("{\n  \"release_ready\": false,\n"), 1),
		"format":        append([]byte(" "), encoded...),
	} {
		t.Run(label, func(t *testing.T) {
			if err := verifyNextInventory(encoded, altered); err == nil {
				t.Fatal("altered inventory accepted")
			}
		})
	}
	// A partial set is useful independently; Setup and DMG are not required.
	for label, names := range map[string][]string{"empty": nil, "duplicate": {name, name}, "outside": {"../" + name}, "absolute": {path}, "stable": {"AgentDock-macos-universal.zip"}, "missing": {"AgentDock-Next-macos-arm64.dmg"}} {
		t.Run(label, func(t *testing.T) {
			if _, err := inventoryNextArtifacts(m, root, "darwin/arm64", names); err == nil {
				t.Fatal("unsafe input accepted")
			}
		})
	}
	if _, err := inventoryNextArtifacts(m, root, "windows/arm64", []string{name}); err == nil {
		t.Fatal("wrong platform accepted")
	}
	if err := os.WriteFile(path, []byte("not a zip"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := inventoryNextArtifacts(m, root, "darwin/arm64", []string{name}); err == nil {
		t.Fatal("wrong type accepted")
	}
}

func TestNextArtifactsTypesAndArchitecture(t *testing.T) {
	root, m, _ := candidateFixture(t)
	for _, arch := range []string{"amd64", "arm64"} {
		// Minimal deterministic PE header fixture; never execute it.
		data := make([]byte, 256)
		copy(data, "MZ")
		binary.LittleEndian.PutUint32(data[0x3c:], 128)
		copy(data[128:], "PE\x00\x00")
		machine := uint16(0x8664)
		if arch == "arm64" {
			machine = 0xaa64
		}
		binary.LittleEndian.PutUint16(data[132:], machine)
		binary.LittleEndian.PutUint16(data[150:], 2)
		for _, name := range []string{"agentdock-next-core.exe", "agentdock-next-arbiter.exe", "AgentDock-NextSetup-" + arch + ".exe"} {
			if err := os.WriteFile(filepath.Join(root, name), data, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := inventoryNextArtifacts(m, root, "windows/"+arch, []string{name}); err != nil {
				t.Fatal(err)
			}
		}
		other := "arm64"
		if arch == "arm64" {
			other = "amd64"
		}
		if _, err := inventoryNextArtifacts(m, root, "windows/"+other, []string{"agentdock-next-core.exe"}); err == nil {
			t.Fatal("wrong PE architecture accepted")
		}
	}
	header := make([]byte, 32)
	binary.LittleEndian.PutUint32(header, 0xfeedfacf)
	binary.LittleEndian.PutUint32(header[4:], 0x100000c)
	binary.LittleEndian.PutUint32(header[12:], 2)
	machName := "agentdock-next-core"
	if err := os.WriteFile(filepath.Join(root, machName), header, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := inventoryNextArtifacts(m, root, "darwin/arm64", []string{machName}); err != nil {
		t.Fatal(err)
	}
	binary.LittleEndian.PutUint32(header[4:], 0x1000007)
	if err := os.WriteFile(filepath.Join(root, machName), header, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := inventoryNextArtifacts(m, root, "darwin/arm64", []string{machName}); err == nil {
		t.Fatal("Intel Mach-O accepted")
	}
	data := make([]byte, 512)
	copy(data, "koly")
	name := "AgentDock-Next-macos-arm64.dmg"
	if err := os.WriteFile(filepath.Join(root, name), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := inventoryNextArtifacts(m, root, "darwin/arm64", []string{name}); err != nil {
		t.Fatal(err)
	}
	if nextArtifactType("darwin/amd64", name) != "" || nextArtifactType("windows/amd64", "agentdock.exe") != "" {
		t.Fatal("unsupported or Stable name allowed")
	}
}

func TestNextArtifactsNoSymlinks(t *testing.T) {
	root, m, _ := candidateFixture(t)
	name := "AgentDock-Next-macos-arm64.dmg"
	if err := os.Symlink(filepath.Join(root, "missing"), filepath.Join(root, name)); err != nil {
		// Windows may lack symlink privileges; use a directory instead and still
		// prove nonregular files fail. Native macOS/Linux exercise actual links.
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := inventoryNextArtifacts(m, root, "darwin/arm64", []string{name}); err == nil {
		t.Fatal("nonregular file accepted")
	}
	link := filepath.Join(root, "linked-root")
	if err := os.Symlink(root, link); err == nil {
		if _, err := inventoryNextArtifacts(m, link, "darwin/arm64", []string{name}); err == nil {
			t.Fatal("symlink root accepted")
		}
	}
}

func TestNextArtifactsCLIRejectsMissingInputs(t *testing.T) {
	for _, args := range [][]string{nil, {"--platform", "darwin/arm64"}, {"--unknown"}, {"--manifest", strings.Repeat("x", 40)}} {
		if err := runNextArtifacts(args, &bytes.Buffer{}); err == nil {
			t.Fatal("insufficient inputs accepted")
		}
	}
}
