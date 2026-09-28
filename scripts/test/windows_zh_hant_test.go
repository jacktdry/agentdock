package scripts

import (
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestWindowsTraditionalChineseResources(t *testing.T) {
	root := filepath.Join("..", "..", "desktop", "windows", "control-panel", "Resources")
	read := func(name string) map[string]string {
		t.Helper()
		file, err := os.Open(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		decoder := xml.NewDecoder(file)
		values := map[string]string{}
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			start, ok := token.(xml.StartElement)
			if !ok || start.Name.Local != "data" {
				continue
			}
			var key string
			for _, attr := range start.Attr {
				if attr.Name.Local == "name" {
					key = attr.Value
				}
			}
			var entry struct {
				Value string `xml:"value"`
			}
			if err := decoder.DecodeElement(&entry, &start); err != nil {
				t.Fatal(err)
			}
			if _, exists := values[key]; exists {
				t.Fatalf("%s has duplicate key %s", name, key)
			}
			values[key] = entry.Value
		}
		return values
	}
	english := read("UiStrings.resx")
	traditional := read("UiStrings.zh-TW.resx")
	if len(traditional) != len(english) {
		t.Fatalf("Traditional Chinese has %d keys; English has %d", len(traditional), len(english))
	}
	format := regexp.MustCompile("\\{\\d+(?::[^}]+)?\\}")
	for key, original := range english {
		translated, ok := traditional[key]
		if !ok || strings.TrimSpace(translated) == "" {
			t.Errorf("missing or empty Traditional Chinese key %s", key)
			continue
		}
		want, got := format.FindAllString(original, -1), format.FindAllString(translated, -1)
		sort.Strings(want)
		sort.Strings(got)
		if strings.Join(want, ",") != strings.Join(got, ",") {
			t.Errorf("%s format placeholders: English %v, Traditional Chinese %v", key, want, got)
		}
	}
	for key := range traditional {
		if _, ok := english[key]; !ok {
			t.Errorf("unexpected Traditional Chinese key %s", key)
		}
	}
	for key, want := range map[string]string{
		"Port": "連接埠", "Service": "服務", "Restart": "重新啟動",
		"OpenConfigFolder": "開啟設定資料夾", "TraditionalChinese": "繁體中文",
	} {
		if traditional[key] != want {
			t.Errorf("%s=%q; want %q", key, traditional[key], want)
		}
	}
}

func TestWindowsTraditionalChineseRoutingContracts(t *testing.T) {
	root := filepath.Join("..", "..", "desktop", "windows")
	checks := map[string][]string{
		filepath.Join("control-panel", "MainWindow.xaml"): {
			"Content=\"{local:Loc TraditionalChinese}\" Tag=\"zh-TW\"",
		},
		filepath.Join("control-panel", "Localization", "UiText.cs"): {
			"TraditionalChinesePreference = \"zh-TW\"",
			"TraditionalChinesePreference => TraditionalChinesePreference",
			"\"zh-tw\" or \"zh-hk\" or \"zh-mo\" or \"zh-hant\"",
			"locale.StartsWith(\"zh-hant-\"",
		},
		filepath.Join("tray", "locale_windows.go"): {
			"case \"zh-TW\":",
			"return trayTextTraditionalChinese",
			"isTraditionalChineseLocale(systemLocale)",
			"\"zh-tw\" || locale == \"zh-hk\" || locale == \"zh-mo\" || locale == \"zh-hant\"",
		},
	}
	for path, wants := range checks {
		data, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range wants {
			if !strings.Contains(string(data), want) {
				t.Errorf("%s missing %q", path, want)
			}
		}
	}
}

func TestWindowsTraditionalChineseTrayTextMatchesEnglishFields(t *testing.T) {
	path := filepath.Join("..", "..", "desktop", "windows", "tray", "locale_windows.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	fields := func(name string) map[string]string {
		t.Helper()
		prefix := "var " + name + " = trayText{"
		start := strings.Index(source, prefix)
		if start < 0 {
			t.Fatalf("missing tray text %s", name)
		}
		end := strings.Index(source[start:], "\n}")
		if end < 0 {
			t.Fatalf("unterminated tray text %s", name)
		}
		block := source[start : start+end]
		matches := regexp.MustCompile("(?m)^\\s*([A-Z][A-Za-z]+):\\s*\"([^\"]*)\"").FindAllStringSubmatch(block, -1)
		result := map[string]string{}
		for _, match := range matches {
			if _, exists := result[match[1]]; exists || match[2] == "" {
				t.Errorf("%s has duplicate or empty field %s", name, match[1])
			}
			result[match[1]] = match[2]
		}
		return result
	}
	english, traditional := fields("trayTextEnglish"), fields("trayTextTraditionalChinese")
	if len(english) != len(traditional) {
		t.Errorf("English tray has %d fields; Traditional Chinese has %d", len(english), len(traditional))
	}
	format := regexp.MustCompile("%[vsd]")
	for field, original := range english {
		translated, ok := traditional[field]
		if !ok {
			t.Errorf("Traditional Chinese tray missing %s", field)
			continue
		}
		if strings.Join(format.FindAllString(original, -1), ",") != strings.Join(format.FindAllString(translated, -1), ",") {
			t.Errorf("Traditional Chinese tray %s has different format verbs", field)
		}
	}
}

func TestWindowsTraditionalChineseInstallerMessages(t *testing.T) {
	root := filepath.Join("..", "..", "packaging", "windows")
	setup, err := os.ReadFile(filepath.Join(root, "AgentDock.iss"))
	if err != nil {
		t.Fatal(err)
	}
	for _, declaration := range []string{
		"Name: \"english\"; MessagesFile: \"compiler:Default.isl\"",
		"Name: \"chinesesimplified\"; MessagesFile: \"compiler:Default.isl, languages\\ChineseSimplified.isl\"",
		"Name: \"chinesetraditional\"; MessagesFile: \"compiler:Default.isl, languages\\ChineseTraditional.isl\"",
	} {
		if !strings.Contains(string(setup), declaration) {
			t.Errorf("installer missing language declaration %q", declaration)
		}
	}
	base, err := os.ReadFile(filepath.Join(root, "languages", "ChineseTraditional.isl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"LanguageName=繁體中文", "LanguageID=$0404", "LanguageCodePage=950",
		"ButtonNext=下一步", "WelcomeLabel1=歡迎使用", "FinishedHeadingLabel=[name] 安裝完成",
		"ConfirmUninstall=確定要完整移除",
	} {
		if !strings.Contains(string(base), want) {
			t.Errorf("Traditional Chinese base messages missing %q", want)
		}
	}
	simplified, err := os.ReadFile(filepath.Join(root, "languages", "ChineseSimplified.isl"))
	if err != nil {
		t.Fatal(err)
	}
	baseKeys := func(data []byte) map[string]string {
		result := map[string]string{}
		inMessages := false
		for _, line := range strings.Split(string(data), "\n") {
			if strings.TrimSpace(line) == "[Messages]" {
				inMessages = true
				continue
			}
			if !inMessages || !strings.Contains(line, "=") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if _, exists := result[parts[0]]; exists {
				t.Errorf("duplicate installer base message %s", parts[0])
			}
			result[parts[0]] = parts[1]
		}
		return result
	}
	simplifiedKeys, traditionalKeys := baseKeys(simplified), baseKeys(base)
	placeholders := regexp.MustCompile("%[1-9n]|\\[[^]]+\\]")
	for key := range simplifiedKeys {
		translated, ok := traditionalKeys[key]
		if !ok {
			t.Errorf("Traditional Chinese base messages missing %s", key)
			continue
		}
		want := placeholders.FindAllString(simplifiedKeys[key], -1)
		got := placeholders.FindAllString(translated, -1)
		sort.Strings(want)
		sort.Strings(got)
		if strings.Join(want, ",") != strings.Join(got, ",") {
			t.Errorf("%s base message placeholders: Simplified %v, Traditional %v", key, want, got)
		}
	}
	if len(simplifiedKeys) != len(traditionalKeys) {
		t.Errorf("Traditional Chinese base messages: %d keys; Simplified Chinese: %d", len(traditionalKeys), len(simplifiedKeys))
	}
	messages, err := os.ReadFile(filepath.Join(root, "includes", "messages.iss"))
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]map[string]bool{}
	for _, line := range strings.Split(string(messages), "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		for _, language := range []string{"english", "chinesesimplified", "chinesetraditional"} {
			if strings.HasPrefix(parts[0], language+".") {
				if keys[language] == nil {
					keys[language] = map[string]bool{}
				}
				key := strings.TrimPrefix(parts[0], language+".")
				if keys[language][key] {
					t.Errorf("duplicate %s installer message %s", language, key)
				}
				keys[language][key] = true
			}
		}
	}
	for key := range keys["english"] {
		if !keys["chinesetraditional"][key] {
			t.Errorf("missing Traditional Chinese installer message %s", key)
		}
	}
	if len(keys["chinesetraditional"]) != len(keys["english"]) {
		t.Errorf("Traditional Chinese custom installer messages: %d keys; English: %d", len(keys["chinesetraditional"]), len(keys["english"]))
	}
}
