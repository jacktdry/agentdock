# AgentDock Localization

This directory is the human-maintained localization source for the custom desktop line.

## Source of truth

- `manifest.yaml`: locale metadata, fallback, aliases, and native platform tags.
- `locales/<locale>.yaml`: semantic message keys and ICU MessageFormat values.
- `glossary.yaml`: product terminology and translation guidance.
- `schema/messages.schema.json`: editor/schema contract for message catalogs.
- `generated/`: deterministic build artifacts. Do not edit these files by hand.
- `migration-inventory.json`: current native-resource parity report. It is an inventory, not an automatic semantic merge.

English (`en`) is the canonical source locale and fallback.

## Contributor workflow

To update an existing translation:

1. Edit only the value in `i18n/locales/<locale>.yaml`.
2. Keep the semantic key unchanged.
3. Keep every ICU argument name and argument kind compatible with English.
4. Run:

```bash
make i18n-generate
make i18n-check
make i18n-coverage
```

To add a locale:

1. Copy `i18n/locales/en.yaml` to the BCP-47-compatible locale code.
2. Translate values without changing keys.
3. Add one entry to `manifest.yaml` with native name, direction, status, fallback, aliases, and platform tags.
4. Start as `draft` or `beta` if coverage is incomplete.
5. Run the commands above and commit source plus generated artifacts.

No Swift enum, C# locale constant, Vue locale list, Python locale tuple, `.strings`, or `.resx` list should be edited merely to register a new Shared UI locale.

## Strict YAML profile

Locale catalogs are intentionally not general-purpose YAML.

- The root is one flat mapping.
- Keys use semantic namespaces such as `common.*`, `runtime.*`, `activity.*`.
- Message values must be double-quoted strings.
- Anchors, aliases, merge keys, custom tags, mappings/sequences as values, and block/folded scalars are rejected.
- Use explicit `\n` inside a quoted value for a line break.
- Unknown keys are rejected.
- `stable` locales must cover every English source key.

## ICU MessageFormat

Arguments are part of the API. For example:

```yaml
activity.summary_active: "Activity stream active. {delivered} delivered, {dropped} dropped."
```

Translations must preserve `delivered` and `dropped`.

Plural and select forms are supported by the validator:

```yaml
files.count: "{count, plural, =0 {No files} one {One file} other {# files}}"
```

Plural/select messages must contain an `other` branch. The validator compares argument names and kinds across locales.

## Generated assets

`go run ./tools/i18n generate` produces:

- Shared Vue/TypeScript catalog and locale manifest.
- Swift locale descriptors for macOS.
- C# locale descriptors for Windows.
- deterministic macOS `.strings` and Windows `.resx` bridge artifacts under `i18n/generated/`.
- coverage JSON.
- legacy native migration inventory.

The existing AppKit/WPF message catalogs remain runtime resources during M3. The bridge artifacts prove deterministic native generation without pretending that legacy English-phrase keys and Windows identifier keys are already semantically migrated.

## Review rules

Do not accept a change when:

- a stable locale loses a key;
- an ICU argument disappears or changes type;
- generated files are stale;
- a Shared UI user-facing literal bypasses the catalog;
- a locale adds its own unreviewed key;
- a translation tool guesses that two legacy native keys are semantically identical and drops one.

Machine-readable protocol names, debug-only logs, and test fixtures are not translation messages.
