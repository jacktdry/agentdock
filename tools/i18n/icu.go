package main

import (
	"fmt"
	"strings"
	"unicode"
)

func parseICUSignature(message string) (map[string]string, error) {
	signature := map[string]string{}
	if err := scanICUMessage(message, signature); err != nil {
		return nil, err
	}
	return signature, nil
}

func scanICUMessage(message string, signature map[string]string) error {
	for i := 0; i < len(message); {
		switch message[i] {
		case '\'':
			next, err := skipICUQuoted(message, i)
			if err != nil {
				return err
			}
			i = next
		case '{':
			end, err := matchingBrace(message, i)
			if err != nil {
				return err
			}
			if err := parseICUArgument(message[i+1:end], signature); err != nil {
				return err
			}
			i = end + 1
		case '}':
			return fmt.Errorf("unexpected closing brace at byte %d", i)
		case '<':
			if i+1 < len(message) && (unicode.IsLetter(rune(message[i+1])) ||
				(message[i+1] == '/' && i+2 < len(message) && unicode.IsLetter(rune(message[i+2])))) {
				return fmt.Errorf("ICU rich-text tags are not supported; quote literal angle brackets")
			}
			i++
		default:
			i++
		}
	}
	return nil
}

func skipICUQuoted(message string, start int) (int, error) {
	if start+1 >= len(message) {
		return start + 1, nil
	}
	if message[start+1] == '\'' {
		return start + 2, nil
	}
	if !strings.ContainsRune("{}#<>", rune(message[start+1])) {
		// ICU apostrophe-friendly mode treats ordinary apostrophes, such as
		// "Don't", as literal text. Quoting starts only before syntax chars.
		return start + 1, nil
	}
	for i := start + 1; i < len(message); i++ {
		if message[i] != '\'' {
			continue
		}
		if i+1 < len(message) && message[i+1] == '\'' {
			i++
			continue
		}
		return i + 1, nil
	}
	return 0, fmt.Errorf("unterminated ICU apostrophe quote at byte %d", start)
}

func matchingBrace(message string, start int) (int, error) {
	depth := 0
	for i := start; i < len(message); i++ {
		if message[i] == '\'' {
			next, err := skipICUQuoted(message, i)
			if err != nil {
				return 0, err
			}
			i = next - 1
			continue
		}
		switch message[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i, nil
			}
			if depth < 0 {
				return 0, fmt.Errorf("unexpected closing brace at byte %d", i)
			}
		}
	}
	return 0, fmt.Errorf("unclosed ICU argument at byte %d", start)
}

func parseICUArgument(content string, signature map[string]string) error {
	parts := splitTopLevel(content, ',')
	if len(parts) == 0 {
		return fmt.Errorf("empty ICU argument")
	}
	name := strings.TrimSpace(parts[0])
	if !validArgumentName(name) {
		return fmt.Errorf("invalid ICU argument name %q", name)
	}
	kind := "simple"
	if len(parts) > 1 {
		kind = strings.TrimSpace(parts[1])
		if kind == "" {
			return fmt.Errorf("ICU argument %q has an empty kind", name)
		}
		if kind == "simple" {
			return fmt.Errorf("ICU argument %q must omit the explicit simple type", name)
		}
	}
	switch kind {
	case "simple", "number", "date", "time", "plural", "selectordinal", "select":
	default:
		return fmt.Errorf("ICU argument %q uses unsupported kind %q", name, kind)
	}
	if (kind == "number" || kind == "date" || kind == "time") && len(parts) > 2 {
		return fmt.Errorf("ICU %s argument %q styles are not supported by the canonical subset", kind, name)
	}
	if previous, ok := signature[name]; ok && previous != kind {
		return fmt.Errorf("ICU argument %q changes kind from %q to %q", name, previous, kind)
	}
	signature[name] = kind

	if kind == "plural" || kind == "selectordinal" || kind == "select" {
		if len(parts) < 3 {
			return fmt.Errorf("ICU %s argument %q has no options", kind, name)
		}
		style := strings.TrimSpace(strings.Join(parts[2:], ","))
		return parseICUOptions(style, kind, signature)
	}
	return nil
}

func parseICUOptions(style, kind string, signature map[string]string) error {
	i := 0
	hasOther := false
	seenSelectors := map[string]bool{}
	offsetSeen := false
	for i < len(style) {
		for i < len(style) && unicode.IsSpace(rune(style[i])) {
			i++
		}
		if i >= len(style) {
			break
		}
		if strings.HasPrefix(style[i:], "offset:") {
			if kind == "select" {
				return fmt.Errorf("ICU select arguments do not support offset")
			}
			if offsetSeen {
				return fmt.Errorf("ICU %s argument defines offset more than once", kind)
			}
			offsetSeen = true
			i += len("offset:")
			start := i
			for i < len(style) && style[i] >= '0' && style[i] <= '9' {
				i++
			}
			if start == i {
				return fmt.Errorf("ICU %s offset requires a number", kind)
			}
			continue
		}
		start := i
		for i < len(style) && !unicode.IsSpace(rune(style[i])) && style[i] != '{' {
			i++
		}
		selector := strings.TrimSpace(style[start:i])
		if selector == "" {
			return fmt.Errorf("ICU %s option is missing a selector", kind)
		}
		for i < len(style) && unicode.IsSpace(rune(style[i])) {
			i++
		}
		if i >= len(style) || style[i] != '{' {
			return fmt.Errorf("ICU %s selector %q is missing a message body", kind, selector)
		}
		end, err := matchingBrace(style, i)
		if err != nil {
			return err
		}
		if seenSelectors[selector] {
			return fmt.Errorf("ICU %s selector %q is duplicated", kind, selector)
		}
		seenSelectors[selector] = true
		if selector == "other" {
			hasOther = true
		}
		if kind == "select" && strings.HasPrefix(selector, "=") {
			return fmt.Errorf("ICU select selector %q must not use exact-number syntax", selector)
		}
		if kind != "select" && strings.HasPrefix(selector, "=") {
			if len(selector) == 1 {
				return fmt.Errorf("ICU %s exact-number selector is missing a number", kind)
			}
			for _, r := range selector[1:] {
				if !unicode.IsDigit(r) {
					return fmt.Errorf("ICU %s selector %q has an invalid exact number", kind, selector)
				}
			}
		}
		if err := scanICUMessage(style[i+1:end], signature); err != nil {
			return err
		}
		i = end + 1
	}
	if !hasOther {
		return fmt.Errorf("ICU %s argument must define an other option", kind)
	}
	return nil
}

func splitTopLevel(value string, separator byte) []string {
	parts := []string{}
	start := 0
	depth := 0
	for i := 0; i < len(value); i++ {
		if value[i] == '\'' {
			next, err := skipICUQuoted(value, i)
			if err == nil {
				i = next - 1
			}
			continue
		}
		switch value[i] {
		case '{':
			depth++
		case '}':
			if depth > 0 {
				depth--
			}
		default:
			if value[i] == separator && depth == 0 {
				parts = append(parts, value[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, value[start:])
	return parts
}

func validArgumentName(value string) bool {
	if value == "" {
		return false
	}
	for i, r := range value {
		if unicode.IsLetter(r) || r == '_' || (i > 0 && unicode.IsDigit(r)) {
			continue
		}
		return false
	}
	return true
}
