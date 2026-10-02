package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	templateTextPattern  = regexp.MustCompile(`>([^<>{]*[A-Za-z][^<>{]*)<`)
	attributeTextPattern = regexp.MustCompile(`\b(?:aria-label|title|placeholder)="([A-Za-z][^"]*[A-Za-z])"`)
	scriptStringPattern  = regexp.MustCompile(`['"]([A-Z][A-Za-z0-9 ,.;:!?—·/-]{5,})['"]`)
)

func scanSharedUIHardcoded(root string) error {
	sourceRoot := filepath.Join(root, "desktop/shared-poc/frontend/src")
	var issues []string
	err := filepath.WalkDir(sourceRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "i18n" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".vue" && ext != ".ts" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(root, path)
		text := string(data)
		if ext == ".vue" {
			template := between(text, "<template>", "</template>")
			for _, match := range templateTextPattern.FindAllStringSubmatch(template, -1) {
				candidate := strings.TrimSpace(match[1])
				if isHardcodedCandidate(candidate) {
					issues = append(issues, fmt.Sprintf("%s: template text %q", filepath.ToSlash(relative), candidate))
				}
			}
			for _, indexes := range attributeTextPattern.FindAllStringSubmatchIndex(template, -1) {
				if indexes[0] > 0 && template[indexes[0]-1] == ':' {
					continue
				}
				candidate := strings.TrimSpace(template[indexes[2]:indexes[3]])
				if isHardcodedCandidate(candidate) {
					issues = append(issues, fmt.Sprintf("%s: user-facing attribute %q", filepath.ToSlash(relative), candidate))
				}
			}
		}
		script := text
		if ext == ".vue" {
			script = between(text, "<script setup lang=\"ts\">", "</script>")
		}
		for _, match := range scriptStringPattern.FindAllStringSubmatch(script, -1) {
			candidate := strings.TrimSpace(match[1])
			if isHardcodedCandidate(candidate) {
				issues = append(issues, fmt.Sprintf("%s: script string %q", filepath.ToSlash(relative), candidate))
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(issues) > 0 {
		sort.Strings(issues)
		return fmt.Errorf("hard-coded shared UI text detected:\n- %s", strings.Join(issues, "\n- "))
	}
	return nil
}

func between(value, start, end string) string {
	startIndex := strings.Index(value, start)
	if startIndex < 0 {
		return ""
	}
	startIndex += len(start)
	endIndex := strings.Index(value[startIndex:], end)
	if endIndex < 0 {
		return value[startIndex:]
	}
	return value[startIndex : startIndex+endIndex]
}

func isHardcodedCandidate(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, "{{") || strings.HasPrefix(value, ":") {
		return false
	}
	allowed := map[string]bool{
		"M3":          true,
		"ms":          true,
		"Nexus":       true,
		"Desktop API": false,
	}
	if allowed[value] {
		return false
	}
	if strings.HasPrefix(value, "M3 ") || strings.HasPrefix(value, "M2 ") {
		return true
	}
	return strings.ContainsAny(value, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
}
