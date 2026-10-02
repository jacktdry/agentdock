package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	manifestPath           = "i18n/manifest.yaml"
	localesDir             = "i18n/locales"
	migrationInventoryPath = "i18n/migration-inventory.json"
)

var (
	localeCodePattern = regexp.MustCompile(`^[A-Za-z]{2,3}(?:-[A-Za-z0-9]{2,8})*$`)
	messageKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[.][a-z0-9_]+)+$`)
)

type Manifest struct {
	Version       int      `yaml:"version" json:"version"`
	SourceLocale  string   `yaml:"sourceLocale" json:"sourceLocale"`
	DefaultLocale string   `yaml:"defaultLocale" json:"defaultLocale"`
	Locales       []Locale `yaml:"locales" json:"locales"`
}

type Locale struct {
	Code       string         `yaml:"code" json:"code"`
	NativeName string         `yaml:"nativeName" json:"nativeName"`
	Direction  string         `yaml:"direction" json:"direction"`
	Status     string         `yaml:"status" json:"status"`
	Fallback   string         `yaml:"fallback" json:"fallback"`
	Aliases    []string       `yaml:"aliases" json:"aliases"`
	Platforms  LocalePlatform `yaml:"platforms" json:"platforms"`
}

type LocalePlatform struct {
	MacOS   string `yaml:"macos" json:"macos"`
	Windows string `yaml:"windows" json:"windows"`
}

type CoverageRow struct {
	Code       string  `json:"code"`
	Status     string  `json:"status"`
	Translated int     `json:"translated"`
	Total      int     `json:"total"`
	Percent    float64 `json:"percent"`
}

type Project struct {
	Manifest Manifest
	Catalogs map[string]map[string]string
	Source   map[string]string
	Coverage []CoverageRow
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if fileExists(filepath.Join(dir, "go.mod")) && fileExists(filepath.Join(dir, manifestPath)) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("repository root not found")
		}
		dir = parent
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func loadProject(root string) (*Project, error) {
	if err := validateSchemaContract(root); err != nil {
		return nil, err
	}
	manifest, err := loadManifest(filepath.Join(root, manifestPath))
	if err != nil {
		return nil, err
	}
	if err := validateManifest(manifest); err != nil {
		return nil, err
	}

	catalogs := make(map[string]map[string]string, len(manifest.Locales))
	for _, locale := range manifest.Locales {
		path := filepath.Join(root, localesDir, locale.Code+".yaml")
		catalog, err := loadStrictCatalog(path)
		if err != nil {
			return nil, err
		}
		catalogs[locale.Code] = catalog
	}
	source := catalogs[manifest.SourceLocale]
	if source == nil {
		return nil, fmt.Errorf("source locale %q has no catalog", manifest.SourceLocale)
	}
	coverage, err := validateCatalogs(manifest, catalogs, source)
	if err != nil {
		return nil, err
	}
	return &Project{Manifest: manifest, Catalogs: catalogs, Source: source, Coverage: coverage}, nil
}

func validateSchemaContract(root string) error {
	path := filepath.Join(root, "i18n/schema/messages.schema.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var schema struct {
		Type          string `json:"type"`
		PropertyNames struct {
			Pattern string `json:"pattern"`
		} `json:"propertyNames"`
		AdditionalProperties struct {
			Type string `json:"type"`
		} `json:"additionalProperties"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		return fmt.Errorf("i18n/schema/messages.schema.json: %w", err)
	}
	if schema.Type != "object" || schema.AdditionalProperties.Type != "string" || schema.PropertyNames.Pattern != messageKeyPattern.String() {
		return fmt.Errorf("i18n/schema/messages.schema.json: schema drifted from the first-party catalog validator")
	}
	return nil
}

func loadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("%s: %w", manifestPath, err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("%s: %w", manifestPath, err)
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Manifest{}, fmt.Errorf("%s: multiple YAML documents are forbidden", manifestPath)
		}
		return Manifest{}, fmt.Errorf("%s: trailing YAML document: %w", manifestPath, err)
	}
	return manifest, nil
}

func validateManifest(manifest Manifest) error {
	if manifest.Version != 1 {
		return fmt.Errorf("%s: unsupported version %d", manifestPath, manifest.Version)
	}
	if manifest.SourceLocale == "" || manifest.DefaultLocale == "" {
		return fmt.Errorf("%s: sourceLocale and defaultLocale are required", manifestPath)
	}
	byCode := map[string]Locale{}
	aliases := map[string]string{}
	macOSTags := map[string]string{}
	windowsTags := map[string]string{}
	for _, locale := range manifest.Locales {
		if !localeCodePattern.MatchString(locale.Code) {
			return fmt.Errorf("%s: invalid locale code %q", manifestPath, locale.Code)
		}
		if _, exists := byCode[locale.Code]; exists {
			return fmt.Errorf("%s: duplicate locale %q", manifestPath, locale.Code)
		}
		if strings.TrimSpace(locale.NativeName) == "" {
			return fmt.Errorf("%s: locale %q has empty nativeName", manifestPath, locale.Code)
		}
		if locale.Direction != "ltr" && locale.Direction != "rtl" {
			return fmt.Errorf("%s: locale %q has invalid direction %q", manifestPath, locale.Code, locale.Direction)
		}
		if locale.Status != "draft" && locale.Status != "beta" && locale.Status != "stable" {
			return fmt.Errorf("%s: locale %q has invalid status %q", manifestPath, locale.Code, locale.Status)
		}
		if !localeCodePattern.MatchString(locale.Platforms.MacOS) || !localeCodePattern.MatchString(locale.Platforms.Windows) {
			return fmt.Errorf("%s: locale %q must define valid macos/windows locale tags", manifestPath, locale.Code)
		}
		for platform, tagOwners := range map[string]struct {
			tag    string
			owners map[string]string
		}{
			"macos":   {tag: locale.Platforms.MacOS, owners: macOSTags},
			"windows": {tag: locale.Platforms.Windows, owners: windowsTags},
		} {
			key := strings.ToLower(tagOwners.tag)
			if prior, exists := tagOwners.owners[key]; exists {
				return fmt.Errorf("%s: %s platform tag %q is shared by locales %q and %q", manifestPath, platform, tagOwners.tag, prior, locale.Code)
			}
			tagOwners.owners[key] = locale.Code
		}
		byCode[locale.Code] = locale
		for _, alias := range append([]string{locale.Code}, locale.Aliases...) {
			if !localeCodePattern.MatchString(alias) {
				return fmt.Errorf("%s: locale %q has invalid alias %q", manifestPath, locale.Code, alias)
			}
			key := strings.ToLower(alias)
			if prior, exists := aliases[key]; exists && prior != locale.Code {
				return fmt.Errorf("%s: locale alias %q belongs to both %q and %q", manifestPath, alias, prior, locale.Code)
			}
			aliases[key] = locale.Code
		}
	}
	if _, ok := byCode[manifest.SourceLocale]; !ok {
		return fmt.Errorf("%s: sourceLocale %q is not declared", manifestPath, manifest.SourceLocale)
	}
	if _, ok := byCode[manifest.DefaultLocale]; !ok {
		return fmt.Errorf("%s: defaultLocale %q is not declared", manifestPath, manifest.DefaultLocale)
	}
	for _, locale := range manifest.Locales {
		if locale.Code == manifest.SourceLocale && locale.Fallback != "" {
			return fmt.Errorf("%s: source locale %q must not declare a fallback", manifestPath, locale.Code)
		}
		if locale.Fallback != "" {
			if _, ok := byCode[locale.Fallback]; !ok {
				return fmt.Errorf("%s: locale %q references unknown fallback %q", manifestPath, locale.Code, locale.Fallback)
			}
		}
		seen := map[string]bool{locale.Code: true}
		next := locale.Fallback
		for next != "" {
			if seen[next] {
				return fmt.Errorf("%s: fallback loop starting at %q", manifestPath, locale.Code)
			}
			seen[next] = true
			next = byCode[next].Fallback
		}
	}
	return nil
}

func loadStrictCatalog(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := decoder.Decode(&doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("%s: multiple YAML documents are forbidden", path)
		}
		return nil, fmt.Errorf("%s: trailing YAML document: %w", path, err)
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s: catalog root must be a mapping", path)
	}
	root := doc.Content[0]
	result := make(map[string]string, len(root.Content)/2)
	for i := 0; i < len(root.Content); i += 2 {
		keyNode, valueNode := root.Content[i], root.Content[i+1]
		if keyNode.Kind != yaml.ScalarNode || keyNode.Tag != "!!str" {
			return nil, fmt.Errorf("%s:%d: catalog key must be a string scalar", path, keyNode.Line)
		}
		if valueNode.Kind != yaml.ScalarNode || valueNode.Tag != "!!str" {
			return nil, fmt.Errorf("%s:%d: %s must have a string value", path, valueNode.Line, keyNode.Value)
		}
		if valueNode.Style&yaml.DoubleQuotedStyle == 0 {
			return nil, fmt.Errorf("%s:%d: %s must use a double-quoted scalar", path, valueNode.Line, keyNode.Value)
		}
		if keyNode.Anchor != "" || valueNode.Anchor != "" || keyNode.Kind == yaml.AliasNode || valueNode.Kind == yaml.AliasNode {
			return nil, fmt.Errorf("%s:%d: anchors and aliases are forbidden", path, keyNode.Line)
		}
		if !messageKeyPattern.MatchString(keyNode.Value) {
			return nil, fmt.Errorf("%s:%d: invalid semantic key %q", path, keyNode.Line, keyNode.Value)
		}
		if _, exists := result[keyNode.Value]; exists {
			return nil, fmt.Errorf("%s:%d: duplicate key %q", path, keyNode.Line, keyNode.Value)
		}
		result[keyNode.Value] = valueNode.Value
	}
	return result, nil
}

func validateCatalogs(manifest Manifest, catalogs map[string]map[string]string, source map[string]string) ([]CoverageRow, error) {
	sourceKeys := sortedKeys(source)
	sourceSignatures := make(map[string]map[string]string, len(source))
	for _, key := range sourceKeys {
		signature, err := parseICUSignature(source[key])
		if err != nil {
			return nil, fmt.Errorf("%s/%s.yaml: %s: %w", localesDir, manifest.SourceLocale, key, err)
		}
		sourceSignatures[key] = signature
	}
	rows := make([]CoverageRow, 0, len(manifest.Locales))
	for _, locale := range manifest.Locales {
		catalog := catalogs[locale.Code]
		for key, value := range catalog {
			if _, ok := source[key]; !ok {
				return nil, fmt.Errorf("%s/%s.yaml: unknown key %q", localesDir, locale.Code, key)
			}
			signature, err := parseICUSignature(value)
			if err != nil {
				return nil, fmt.Errorf("%s/%s.yaml: %s: %w", localesDir, locale.Code, key, err)
			}
			if !sameSignature(sourceSignatures[key], signature) {
				return nil, fmt.Errorf("%s/%s.yaml: %s ICU arguments differ: source=%v locale=%v", localesDir, locale.Code, key, sourceSignatures[key], signature)
			}
		}
		missing := make([]string, 0)
		for _, key := range sourceKeys {
			if _, ok := catalog[key]; !ok {
				missing = append(missing, key)
			}
		}
		if locale.Status == "stable" && len(missing) > 0 {
			return nil, fmt.Errorf("%s/%s.yaml: stable locale is missing %d key(s): %s", localesDir, locale.Code, len(missing), strings.Join(missing, ", "))
		}
		translated := len(sourceKeys) - len(missing)
		percent := 100.0
		if len(sourceKeys) > 0 {
			percent = float64(translated) * 100 / float64(len(sourceKeys))
		}
		rows = append(rows, CoverageRow{
			Code: locale.Code, Status: locale.Status, Translated: translated, Total: len(sourceKeys), Percent: percent,
		})
	}
	return rows, nil
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sameSignature(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if b[key] != value {
			return false
		}
	}
	return true
}
