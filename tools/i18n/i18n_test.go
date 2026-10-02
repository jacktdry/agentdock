package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseICUSignature(t *testing.T) {
	message := "{count, plural, =0 {No files} one {{owner} has one file} other {{owner} has # files}}"
	got, err := parseICUSignature(message)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"count": "plural", "owner": "simple"}
	if !sameSignature(want, got) {
		t.Fatalf("signature = %#v, want %#v", got, want)
	}
}

func TestParseICUSignatureRequiresOther(t *testing.T) {
	if _, err := parseICUSignature("{count, plural, one {one}}"); err == nil {
		t.Fatal("expected plural without other to fail")
	}
}

func TestStrictCatalogRejectsUnquotedValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "en.yaml")
	if err := os.WriteFile(path, []byte("common.save: Save\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadStrictCatalog(path); err == nil {
		t.Fatal("expected unquoted YAML value to fail")
	}
}

func TestStrictCatalogAcceptsDoubleQuotedValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "en.yaml")
	if err := os.WriteFile(path, []byte("common.save: \"Save\"\nruntime.summary: \"Status: {state}\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	catalog, err := loadStrictCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	if catalog["runtime.summary"] != "Status: {state}" {
		t.Fatalf("unexpected catalog: %#v", catalog)
	}
}

func TestManifestRejectsFallbackLoop(t *testing.T) {
	manifest := Manifest{
		Version:       1,
		SourceLocale:  "en",
		DefaultLocale: "en",
		Locales: []Locale{
			{Code: "en", NativeName: "English", Direction: "ltr", Status: "stable", Platforms: LocalePlatform{MacOS: "en", Windows: "en"}},
			{Code: "fr", NativeName: "Français", Direction: "ltr", Status: "beta", Fallback: "de", Platforms: LocalePlatform{MacOS: "fr", Windows: "fr"}},
			{Code: "de", NativeName: "Deutsch", Direction: "ltr", Status: "beta", Fallback: "fr", Platforms: LocalePlatform{MacOS: "de", Windows: "de"}},
		},
	}
	if err := validateManifest(manifest); err == nil {
		t.Fatal("expected fallback loop to fail")
	}
}

func TestCatalogArgumentKindsMustMatch(t *testing.T) {
	manifest := Manifest{
		Version:       1,
		SourceLocale:  "en",
		DefaultLocale: "en",
		Locales: []Locale{
			{Code: "en", NativeName: "English", Direction: "ltr", Status: "stable", Platforms: LocalePlatform{MacOS: "en", Windows: "en"}},
			{Code: "zh-Hant", NativeName: "繁體中文", Direction: "ltr", Status: "stable", Fallback: "en", Platforms: LocalePlatform{MacOS: "zh-Hant", Windows: "zh-TW"}},
		},
	}
	catalogs := map[string]map[string]string{
		"en":      {"files.count": "{count, plural, one {one} other {#}}"},
		"zh-Hant": {"files.count": "{count}"},
	}
	if _, err := validateCatalogs(manifest, catalogs, catalogs["en"]); err == nil {
		t.Fatal("expected ICU argument-kind mismatch to fail")
	}
}

func TestICUApostropheFriendlyMode(t *testing.T) {
	if _, err := parseICUSignature("Don't stop"); err != nil {
		t.Fatalf("ordinary apostrophe should be literal text: %v", err)
	}
}

func TestICURejectsExplicitSimpleType(t *testing.T) {
	if _, err := parseICUSignature("{name, simple}"); err == nil {
		t.Fatal("expected explicit simple type to fail")
	}
}

func TestICURejectsDuplicatePluralSelector(t *testing.T) {
	if _, err := parseICUSignature("{count, plural, one {one} one {duplicate} other {other}}"); err == nil {
		t.Fatal("expected duplicate plural selector to fail")
	}
}

func TestICURejectsRichTextTags(t *testing.T) {
	if _, err := parseICUSignature("<b>Ready</b>"); err == nil {
		t.Fatal("expected rich-text tag to fail")
	}
}

func TestICURejectsUnsupportedSimpleFormatStyle(t *testing.T) {
	if _, err := parseICUSignature("{count, number, ::currency/USD}"); err == nil {
		t.Fatal("expected number style outside the canonical subset to fail")
	}
}

func TestManifestRejectsMultipleYAMLDocuments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.yaml")
	content := "version: 1\nsourceLocale: en\ndefaultLocale: en\nlocales: []\n---\nversion: 2\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadManifest(path); err == nil {
		t.Fatal("expected multiple manifest YAML documents to fail")
	}
}

func TestStrictCatalogRejectsMultipleYAMLDocuments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "en.yaml")
	content := "common.save: \"Save\"\n---\ncommon.cancel: \"Cancel\"\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadStrictCatalog(path); err == nil {
		t.Fatal("expected multiple catalog YAML documents to fail")
	}
}

func TestManifestRejectsDuplicatePlatformTags(t *testing.T) {
	manifest := Manifest{
		Version:       1,
		SourceLocale:  "en",
		DefaultLocale: "en",
		Locales: []Locale{
			{Code: "en", NativeName: "English", Direction: "ltr", Status: "stable", Platforms: LocalePlatform{MacOS: "en", Windows: "en"}},
			{Code: "fr", NativeName: "Français", Direction: "ltr", Status: "beta", Fallback: "en", Platforms: LocalePlatform{MacOS: "en", Windows: "fr"}},
		},
	}
	if err := validateManifest(manifest); err == nil {
		t.Fatal("expected duplicate macOS platform tag to fail")
	}
}

func TestCheckOutputsRejectsObsoleteGeneratedFile(t *testing.T) {
	root := t.TempDir()
	obsolete := filepath.Join(root, "i18n", "generated", "windows", "M3.obsolete.resx")
	if err := os.MkdirAll(filepath.Dir(obsolete), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(obsolete, []byte("obsolete"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := checkOutputs(root, map[string][]byte{}); err == nil {
		t.Fatal("expected obsolete generated file to fail freshness check")
	}
}
