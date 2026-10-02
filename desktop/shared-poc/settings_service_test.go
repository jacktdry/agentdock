package main

import (
	"path/filepath"
	"testing"
)

func TestSettingsRoundTrip(t *testing.T) {
	service := NewSettingsService(filepath.Join(t.TempDir(), "preferences.json"))
	want := Preferences{BatchIntervalMS: 250, WindowWidth: 1280, WindowHeight: 800}
	if result := service.Save(want); !result.Saved || result.Error != nil {
		t.Fatalf("Save() = %#v", result)
	}
	got := service.Get()
	if got.Error != nil {
		t.Fatalf("Get() error = %#v", got.Error)
	}
	if got.Preferences != want {
		t.Fatalf("Get() = %#v, want %#v", got.Preferences, want)
	}
}

func TestSettingsRejectsInvalidBatchInterval(t *testing.T) {
	service := NewSettingsService(filepath.Join(t.TempDir(), "preferences.json"))
	result := service.Save(Preferences{BatchIntervalMS: 10, WindowWidth: 1080, WindowHeight: 720})
	if result.Error == nil || result.Error.Code != "preferences_invalid" {
		t.Fatalf("expected preferences_invalid, got %#v", result)
	}
}
