package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

const (
	defaultBatchIntervalMS = 100
	minBatchIntervalMS     = 50
	maxBatchIntervalMS     = 1000
)

type SettingsService struct {
	mu   sync.Mutex
	path string
}

func NewSettingsService(path string) *SettingsService {
	if path == "" {
		path = defaultSettingsPath()
	}
	return &SettingsService{path: path}
}

func defaultSettingsPath() string {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "AgentDock", "shared-desktop-poc", "preferences.json")
}

func defaultPreferences() Preferences {
	return Preferences{
		BatchIntervalMS: defaultBatchIntervalMS,
		WindowWidth:     1080,
		WindowHeight:    720,
	}
}

func (s *SettingsService) Get() PreferencesResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	prefs, err := s.readLocked()
	if err != nil {
		return PreferencesResult{Preferences: defaultPreferences(), Error: apiError("preferences_read_failed", err)}
	}
	return PreferencesResult{Preferences: prefs}
}

func (s *SettingsService) Save(prefs Preferences) SavePreferencesResult {
	if err := validatePreferences(prefs); err != nil {
		return SavePreferencesResult{Error: apiError("preferences_invalid", err)}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.writeLocked(prefs); err != nil {
		return SavePreferencesResult{Error: apiError("preferences_write_failed", err)}
	}
	return SavePreferencesResult{Saved: true}
}

func (s *SettingsService) saveWindowSize(width, height int) error {
	if width < 1 || height < 1 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	prefs, err := s.readLocked()
	if err != nil {
		prefs = defaultPreferences()
	}
	prefs.WindowWidth = width
	prefs.WindowHeight = height
	return s.writeLocked(prefs)
}

func (s *SettingsService) readLocked() (Preferences, error) {
	prefs := defaultPreferences()
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return prefs, nil
	}
	if err != nil {
		return prefs, err
	}
	if err := json.Unmarshal(data, &prefs); err != nil {
		return defaultPreferences(), err
	}
	if err := validatePreferences(prefs); err != nil {
		return defaultPreferences(), err
	}
	return prefs, nil
}

func (s *SettingsService) writeLocked(prefs Preferences) error {
	if err := validatePreferences(prefs); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(prefs, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func validatePreferences(prefs Preferences) error {
	if prefs.BatchIntervalMS < minBatchIntervalMS || prefs.BatchIntervalMS > maxBatchIntervalMS {
		return errors.New("batch interval must be between 50 and 1000 milliseconds")
	}
	if prefs.WindowWidth < 1 || prefs.WindowHeight < 1 {
		return errors.New("window dimensions must be positive")
	}
	return nil
}
