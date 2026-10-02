package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const generatedHeader = "GENERATED FILE — DO NOT EDIT"

var generatedOwnedRoots = []string{
	"i18n/generated/macos",
	"i18n/generated/windows",
}

type migrationInventory struct {
	GeneratedFrom string                      `json:"generatedFrom"`
	MacOS         map[string]resourceSnapshot `json:"macOS"`
	Windows       map[string]resourceSnapshot `json:"windows"`
	Notes         []string                    `json:"notes"`
}

type resourceSnapshot struct {
	Path     string   `json:"path"`
	KeyCount int      `json:"keyCount"`
	Missing  []string `json:"missingFromBaseline,omitempty"`
	Extra    []string `json:"extraAgainstBaseline,omitempty"`
}

func generateOutputs(root string, project *Project) (map[string][]byte, error) {
	outputs := map[string][]byte{}

	manifestJSON, err := json.MarshalIndent(project.Manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	catalogJSON, err := json.MarshalIndent(project.Catalogs, "", "  ")
	if err != nil {
		return nil, err
	}
	ts := "// " + generatedHeader + "\n" +
		"export const localeManifest = " + string(manifestJSON) + " as const\n\n" +
		"export const messageCatalogs = " + string(catalogJSON) + " as const\n"
	outputs["desktop/shared-poc/frontend/src/i18n/generated.ts"] = []byte(ts)

	outputs["desktop/macos/AgentDockApp/Sources/GeneratedLocales.swift"] = []byte(renderSwiftLocales(project.Manifest))
	outputs["desktop/windows/control-panel/Localization/GeneratedLocales.cs"] = []byte(renderCSharpLocales(project.Manifest))

	for _, locale := range project.Manifest.Locales {
		catalog := project.Catalogs[locale.Code]
		outputs[filepath.ToSlash(filepath.Join("i18n/generated/macos", locale.Platforms.MacOS+".lproj", "M3.strings"))] =
			[]byte(renderStringsCatalog(catalog))
		name := "M3.resx"
		if locale.Code != project.Manifest.SourceLocale {
			name = "M3." + locale.Platforms.Windows + ".resx"
		}
		outputs[filepath.ToSlash(filepath.Join("i18n/generated/windows", name))] = []byte(renderResxCatalog(catalog))
	}

	coverage, err := marshalJSON(struct {
		SourceLocale string        `json:"sourceLocale"`
		Rows         []CoverageRow `json:"locales"`
	}{SourceLocale: project.Manifest.SourceLocale, Rows: project.Coverage})
	if err != nil {
		return nil, err
	}
	outputs["i18n/generated/coverage.json"] = coverage

	inventory, err := buildMigrationInventory(root)
	if err != nil {
		return nil, err
	}
	inventoryJSON, err := marshalJSON(inventory)
	if err != nil {
		return nil, err
	}
	outputs[migrationInventoryPath] = inventoryJSON

	return outputs, nil
}

func marshalJSON(value any) ([]byte, error) {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(content, '\n'), nil
}

func renderSwiftLocales(manifest Manifest) string {
	var b strings.Builder
	b.WriteString("// " + generatedHeader + "\n")
	b.WriteString("import Foundation\n\n")
	b.WriteString("struct GeneratedLocaleDescriptor: Equatable {\n")
	b.WriteString("    let code: String\n    let nativeName: String\n    let macOSResource: String\n    let aliases: [String]\n}\n\n")
	b.WriteString("enum GeneratedLocales {\n")
	fmt.Fprintf(&b, "    static let sourceLocale = %q\n", manifest.SourceLocale)
	fmt.Fprintf(&b, "    static let defaultLocale = %q\n", manifest.DefaultLocale)
	b.WriteString("    static let supported: [GeneratedLocaleDescriptor] = [\n")
	for _, locale := range manifest.Locales {
		fmt.Fprintf(&b, "        .init(code: %q, nativeName: %q, macOSResource: %q, aliases: %s),\n",
			locale.Code, locale.NativeName, locale.Platforms.MacOS, swiftArray(locale.Aliases))
	}
	b.WriteString("    ]\n}\n")
	return b.String()
}

func swiftArray(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, fmt.Sprintf("%q", value))
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

func renderCSharpLocales(manifest Manifest) string {
	var b strings.Builder
	b.WriteString("// " + generatedHeader + "\n")
	b.WriteString("namespace AgentDock.ControlPanel;\n\n")
	b.WriteString("internal sealed record GeneratedLocaleDescriptor(string Code, string NativeName, string WindowsResource, string[] Aliases);\n\n")
	b.WriteString("internal static class GeneratedLocales\n{\n")
	fmt.Fprintf(&b, "    internal const string SourceLocale = %q;\n", manifest.SourceLocale)
	fmt.Fprintf(&b, "    internal const string DefaultLocale = %q;\n", manifest.DefaultLocale)
	b.WriteString("    internal static readonly GeneratedLocaleDescriptor[] Supported =\n    [\n")
	for _, locale := range manifest.Locales {
		fmt.Fprintf(&b, "        new(%q, %q, %q, [%s]),\n",
			locale.Code, locale.NativeName, locale.Platforms.Windows, csharpArray(locale.Aliases))
	}
	b.WriteString("    ];\n}\n")
	return b.String()
}

func csharpArray(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, fmt.Sprintf("%q", value))
	}
	return strings.Join(quoted, ", ")
}

func renderStringsCatalog(catalog map[string]string) string {
	var b strings.Builder
	b.WriteString("/* " + generatedHeader + " */\n")
	for _, key := range sortedKeys(catalog) {
		fmt.Fprintf(&b, "\"%s\" = \"%s\";\n", escapeAppleString(key), escapeAppleString(catalog[key]))
	}
	return b.String()
}

func escapeAppleString(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	value = strings.ReplaceAll(value, "\n", "\\n")
	return value
}

func renderResxCatalog(catalog map[string]string) string {
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n")
	b.WriteString("<!-- " + generatedHeader + " -->\n<root>\n")
	for _, key := range sortedKeys(catalog) {
		b.WriteString("  <data name=\"" + xmlEscape(key) + "\" xml:space=\"preserve\">\n")
		b.WriteString("    <value>" + xmlEscape(catalog[key]) + "</value>\n")
		b.WriteString("  </data>\n")
	}
	b.WriteString("</root>\n")
	return b.String()
}

func xmlEscape(value string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(value))
	return b.String()
}

func writeOutputs(root string, outputs map[string][]byte) error {
	for _, relative := range generatedOwnedRoots {
		if err := os.RemoveAll(filepath.Join(root, filepath.FromSlash(relative))); err != nil {
			return fmt.Errorf("clean generated i18n root %s: %w", relative, err)
		}
	}

	paths := make([]string, 0, len(outputs))
	for path := range outputs {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, relative := range paths {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, outputs[relative], 0o644); err != nil {
			return err
		}
	}
	return nil
}

func checkOutputs(root string, outputs map[string][]byte) error {
	var stale []string
	for relative, expected := range outputs {
		actual, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil || !bytes.Equal(actual, expected) {
			stale = append(stale, relative)
		}
	}

	for _, ownedRoot := range generatedOwnedRoots {
		absoluteRoot := filepath.Join(root, filepath.FromSlash(ownedRoot))
		err := filepath.WalkDir(absoluteRoot, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			if entry.IsDir() {
				return nil
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			relative = filepath.ToSlash(relative)
			if _, expected := outputs[relative]; !expected {
				stale = append(stale, relative+" (unexpected)")
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("inspect generated i18n root %s: %w", ownedRoot, err)
		}
	}

	if len(stale) > 0 {
		sort.Strings(stale)
		return fmt.Errorf("generated i18n artifacts are stale, missing, or obsolete: %s; run go run ./tools/i18n generate", strings.Join(stale, ", "))
	}
	return nil
}

func buildMigrationInventory(root string) (migrationInventory, error) {
	inventory := migrationInventory{
		GeneratedFrom: "existing native resources; values are inventoried only and are never semantically merged automatically",
		MacOS:         map[string]resourceSnapshot{},
		Windows:       map[string]resourceSnapshot{},
		Notes: []string{
			"macOS legacy Localizable.strings uses English phrases as keys.",
			"Windows legacy UiStrings.resx uses identifier-style keys.",
			"M3 canonical semantic keys remain separate until each native surface is migrated explicitly.",
		},
	}

	macPaths := map[string]string{
		"en":      "desktop/macos/AgentDockApp/Resources/en.lproj/Localizable.strings",
		"zh-Hant": "desktop/macos/AgentDockApp/Resources/zh-Hant.lproj/Localizable.strings",
		"zh-Hans": "desktop/macos/AgentDockApp/Resources/zh-Hans.lproj/Localizable.strings",
	}
	macSets := map[string]map[string]bool{}
	for locale, relative := range macPaths {
		keys, err := parseStringsKeys(filepath.Join(root, relative))
		if err != nil {
			return inventory, err
		}
		macSets[locale] = keys
		inventory.MacOS[locale] = resourceSnapshot{Path: relative, KeyCount: len(keys)}
	}
	for locale, snapshot := range inventory.MacOS {
		snapshot.Missing = setDifference(macSets["en"], macSets[locale])
		snapshot.Extra = setDifference(macSets[locale], macSets["en"])
		inventory.MacOS[locale] = snapshot
	}

	winPaths := map[string]string{
		"en":      "desktop/windows/control-panel/Resources/UiStrings.resx",
		"zh-Hant": "desktop/windows/control-panel/Resources/UiStrings.zh-TW.resx",
		"zh-Hans": "desktop/windows/control-panel/Resources/UiStrings.zh-CN.resx",
	}
	winSets := map[string]map[string]bool{}
	for locale, relative := range winPaths {
		keys, err := parseResxKeys(filepath.Join(root, relative))
		if err != nil {
			return inventory, err
		}
		winSets[locale] = keys
		inventory.Windows[locale] = resourceSnapshot{Path: relative, KeyCount: len(keys)}
	}
	for locale, snapshot := range inventory.Windows {
		snapshot.Missing = setDifference(winSets["en"], winSets[locale])
		snapshot.Extra = setDifference(winSets[locale], winSets["en"])
		inventory.Windows[locale] = snapshot
	}
	return inventory, nil
}

func parseStringsKeys(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	keys := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "\"") {
			continue
		}
		end := strings.Index(line[1:], "\"")
		if end < 0 {
			continue
		}
		keys[line[1:end+1]] = true
	}
	return keys, nil
}

type resxRoot struct {
	Data []resxData `xml:"data"`
}

type resxData struct {
	Name string `xml:"name,attr"`
}

func parseResxKeys(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var root resxRoot
	if err := xml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	keys := make(map[string]bool, len(root.Data))
	for _, item := range root.Data {
		keys[item.Name] = true
	}
	return keys, nil
}

func setDifference(left, right map[string]bool) []string {
	var values []string
	for value := range left {
		if !right[value] {
			values = append(values, value)
		}
	}
	sort.Strings(values)
	return values
}
