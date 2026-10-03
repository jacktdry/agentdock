package desktopruntime

import "testing"

func TestBasicSettingsValidation(t *testing.T) {
	for _, settings := range []BasicSettings{{Port: 0, LogLevel: "info"}, {Port: 65536, LogLevel: "info"}, {Port: 8765, LogLevel: "trace"}} {
		if validateBasicSettings(settings) == nil {
			t.Fatal(settings)
		}
	}
	for _, level := range []string{"debug", "info", "warn", "error"} {
		if err := validateBasicSettings(BasicSettings{Port: 65535, LogLevel: level}); err != nil {
			t.Fatal(err)
		}
	}
}
