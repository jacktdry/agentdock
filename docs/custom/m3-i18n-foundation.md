# M3 i18n Foundation

> Branch: `architecture/i18n`
>
> Base: M2 `9f37ca88`

## Scope

M3 establishes one localization contract that can be consumed by the Shared UI and used to drive locale registration on AppKit and WPF without requiring contributors to edit platform code.

The milestone does **not** claim that every legacy native message has already moved to semantic keys. Existing AppKit `.strings` and WPF `.resx` keys predate the shared contract and are kept as a migration boundary.

## Canonical authoring contract

Human-maintained files live under `i18n/`:

- `manifest.yaml` owns locale registration, fallback, aliases, status, and platform tags.
- `locales/*.yaml` owns semantic ICU messages.
- `glossary.yaml` records product terminology.
- `schema/messages.schema.json` documents the editor/schema shape.

Initial stable locales are:

- `en`
- `zh-Hant`
- `zh-Hans`

The source locale is English. Stable locales fail validation when any canonical message is missing.

## Validation and generation

`tools/i18n` is a repository-local Go tool with four commands:

```bash
go run ./tools/i18n generate
go run ./tools/i18n check
go run ./tools/i18n coverage
go run ./tools/i18n inventory
```

It validates:

- manifest structure and locale aliases;
- fallback references and cycles;
- the restricted YAML profile;
- semantic key syntax;
- unknown and missing keys;
- structural ICU syntax;
- ICU argument-name and argument-kind parity;
- deterministic generated artifacts;
- Shared UI hard-coded user-facing strings.

Generated files are committed so stale output is detectable in CI.

## Shared UI

The Wails/Vue Shared UI consumes generated TypeScript catalogs through `intl-messageformat`.

Locale selection is generated from the manifest and supports system/browser locale detection plus aliases. Explicit locale choices are stored locally; “Follow system” keeps automatic detection active and does not freeze the currently detected locale as a saved override. Fallback behavior remains manifest-driven.

M3 removes Shared UI user-facing literals from the initial Runtime, Desktop API, Settings, and Activity POC panels. Accessibility labels and live-region announcements use the same message contract.

## Native bridge

### macOS

`GeneratedLocales.swift` is generated from the manifest. `UILanguagePreference` no longer owns a hard-coded language enum; it accepts canonical locale codes and legacy aliases such as `zh-TW`.

The macOS packaging script discovers all `.lproj` resource directories instead of naming locales inline.

### Windows

`GeneratedLocales.cs` is generated from the manifest. `UiText` normalizes canonical codes, Windows resource tags, and aliases through those descriptors. The WPF language chooser is populated from the generated locale list.

Existing stored `zh-CN` / `zh-TW` values remain readable and normalize to canonical `zh-Hans` / `zh-Hant`.

## Existing translation migration

Previous Traditional Chinese work was reused rather than retranslated. Before M3, those branches were nearly current:

- macOS differed by one new runtime-analytics key;
- Windows differed by four runtime-analytics keys.

M3 imports the reviewed Traditional Chinese resources and fills only those new keys. After the update:

- macOS: 363 keys in each of en / zh-Hant / zh-Hans.
- Windows Control Panel: 241 keys in each of en / zh-Hant / zh-Hans.

`i18n/migration-inventory.json` records key-count and parity information. It deliberately does not guess semantic equivalence between legacy macOS English-text keys, Windows identifier keys, and the new canonical semantic keys.

## CI and contributor boundary

`make i18n-check` runs the Go unit tests and the repository i18n check. CI invokes this on Linux in addition to existing platform tests.

Contributor documentation lives in `i18n/README.md`. Adding a Shared UI locale requires editing the locale YAML plus manifest, not Swift/C#/Vue/Python locale lists.

## Deferred work

M3 intentionally leaves these for later migration work:

- converting all existing AppKit/WPF runtime messages to canonical semantic keys;
- installer/tray-specific catalog convergence;
- translation-management-service integration;
- runtime locale download or remote catalogs.

These are not required to start M4 because the locale registration and message contract are already stable.
