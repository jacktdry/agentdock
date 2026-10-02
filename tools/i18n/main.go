package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	root, err := findRepoRoot()
	if err != nil {
		fatal(err)
	}
	if len(os.Args) < 2 {
		fatal(fmt.Errorf("usage: go run ./tools/i18n <generate|check|coverage|inventory>"))
	}
	project, err := loadProject(root)
	if err != nil {
		fatal(err)
	}

	switch os.Args[1] {
	case "generate":
		outputs, err := generateOutputs(root, project)
		if err != nil {
			fatal(err)
		}
		if err := writeOutputs(root, outputs); err != nil {
			fatal(err)
		}
		fmt.Printf("generated %d deterministic i18n artifact(s)\n", len(outputs))
	case "check":
		outputs, err := generateOutputs(root, project)
		if err != nil {
			fatal(err)
		}
		if err := checkOutputs(root, outputs); err != nil {
			fatal(err)
		}
		if err := scanSharedUIHardcoded(root); err != nil {
			fatal(err)
		}
		fmt.Printf("i18n check passed: %d locales, %d canonical keys\n", len(project.Manifest.Locales), len(project.Source))
	case "coverage":
		for _, row := range project.Coverage {
			fmt.Printf("%-8s %6.2f%% %s (%d/%d)\n", row.Code, row.Percent, row.Status, row.Translated, row.Total)
		}
	case "inventory":
		inventory, err := buildMigrationInventory(root)
		if err != nil {
			fatal(err)
		}
		content, err := marshalJSON(inventory)
		if err != nil {
			fatal(err)
		}
		path := filepath.Join(root, migrationInventoryPath)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			fatal(err)
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			fatal(err)
		}
		fmt.Printf("wrote %s\n", migrationInventoryPath)
	default:
		fatal(fmt.Errorf("unknown command %q", os.Args[1]))
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "i18n:", err)
	os.Exit(1)
}
