package main

import "testing"

func TestNativeMenuLanguageFromMacPreferences(t *testing.T) {
	cases := []struct{ pref, want string }{
		{"(\n    \"zh-Hant-TW\",\n    \"en-US\"\n)", "zh-Hant"},
		{"(\n    \"zh-Hans-CN\"\n)", "zh-Hans"},
		{"(\n    \"en-US\",\n    \"zh-Hant-TW\"\n)", "en"},
	}
	for _, tc := range cases {
		got := normalizeNativeMenuLanguage(firstAppleLanguage(tc.pref))
		if got != tc.want {
			t.Errorf("lang %q: got %q, want %q", tc.pref, got, tc.want)
		}
	}
}

func TestNativeMenuLanguageVariants(t *testing.T) {
	for raw, want := range map[string]string{
		"zh_TW.UTF-8": "zh-Hant", "zh-HK": "zh-Hant", "zh-Hans-CN": "zh-Hans",
		"zh_CN": "zh-Hans", "en_US.UTF-8": "en", "fr-FR": "en",
	} {
		if got := normalizeNativeMenuLanguage(raw); got != want {
			t.Errorf("normalize %q: %q != %q", raw, got, want)
		}
	}
}

func TestNativeMenuDoesNotClaimServiceHealth(t *testing.T) {
	labels := labelsForNativeMenu("zh-Hant")
	if labels.open != "開啟" || labels.showWindow != "顯示視窗" || labels.quit != "結束介面" || labels.pluginFolder != "選取 Plugin 資料夾" || labels.choose != "選取" {
		t.Fatalf("unexpected zh-Hant: %+v", labels)
	}
	if labelsForNativeMenu("en").quit != "Quit UI" {
		t.Fatal("quit UI should not imply Core stop")
	}
}
