package browser

import (
	"errors"
	"maps"
	"math"
	"strconv"
)

type externalPage struct {
	isolated bool
}

// Only strict structured pages can prove ownership; text never supplies IDs.
func externalPages(result map[string]any) (map[float64]externalPage, error) {
	if err := leaseCallResultError(result); err != nil {
		return nil, err
	}
	s, ok := result["structuredContent"].(map[string]any)
	if !ok {
		return nil, errors.New("missing structuredContent")
	}
	if externalReconnected(s) {
		return nil, errors.New("reconnected or malformed reconnect state")
	}
	pages, ok := s["pages"].([]any)
	if !ok {
		return nil, errors.New("pages must be an array")
	}
	parsed := make(map[float64]externalPage, len(pages))
	for _, entry := range pages {
		p, ok := entry.(map[string]any)
		if !ok {
			return nil, errors.New("invalid page entry")
		}
		id, ok := p["id"].(float64)
		if !ok || math.IsNaN(id) || math.IsInf(id, 0) || id < 0 || math.Trunc(id) != id || id > 9007199254740991 {
			return nil, errors.New("invalid numeric page id")
		}
		if _, exists := parsed[id]; exists {
			return nil, errors.New("duplicate page id")
		}
		if _, ok := p["url"].(string); !ok {
			return nil, errors.New("invalid page url")
		}
		if _, ok := p["title"].(string); !ok {
			return nil, errors.New("invalid page title")
		}
		_, ok = p["selected"].(bool)
		if !ok {
			return nil, errors.New("invalid selected state")
		}
		isolation, isolated := p["isolatedContext"]
		if isolated {
			if name, ok := isolation.(string); !ok || name == "" {
				return nil, errors.New("invalid isolated context")
			}
		}
		parsed[id] = externalPage{isolated: isolated}
	}
	return parsed, nil
}

// The 1.7.0 pin qualifies stable process-local IDs. Selection is not ownership.
func externalNewPageID(baseline map[float64]externalPage, result map[string]any) (float64, string, error) {
	pages, err := externalPages(result)
	if err != nil {
		return 0, "", err
	}
	var id float64
	count := 0
	for candidate := range pages {
		if _, exists := baseline[candidate]; !exists {
			id = candidate
			count++
		}
	}
	if count != 1 || pages[id].isolated {
		return 0, "", errors.New("expected exactly one new non-isolated page")
	}
	return id, strconv.FormatFloat(id, 'f', 0, 64), nil
}

const (
	externalSelectedPageClosedText = "The selected page has been closed. Call list_pages to see open pages."
	externalLastPageCloseText      = "The last open page cannot be closed. It is fine to keep it open."
)

func externalResultText(result map[string]any) []string {
	content, _ := result["content"].([]any)
	texts := make([]string, 0, len(content))
	for _, item := range content {
		block, _ := item.(map[string]any)
		if block["type"] != "text" {
			continue
		}
		if text, ok := block["text"].(string); ok {
			texts = append(texts, text)
		}
	}
	return texts
}

// chrome-devtools-mcp 1.7.0 closes the selected page before ToolHandler builds
// the response. Response construction then calls getSelectedMcpPage and emits
// this exact isError sentinel even though the requested page is already closed.
func externalCloseResultError(result map[string]any) error {
	texts := externalResultText(result)
	for _, text := range texts {
		if text == externalLastPageCloseText {
			return errors.New(text)
		}
	}
	failed, _ := result["isError"].(bool)
	if !failed {
		return nil
	}
	if len(texts) == 1 && texts[0] == externalSelectedPageClosedText {
		return nil
	}
	return leaseCallResultError(result)
}

func externalReconnected(s map[string]any) bool {
	reconnect, present := s["reconnected"]
	return present && reconnect != false
}

// Catalogs also appear in response text. Preserve operation-specific structured
// fields and non-text content; never expose browser-wide page catalogs.
func sanitizeExternalResult(result map[string]any) map[string]any {
	clean := maps.Clone(result)
	for _, key := range []string{"pages", "extensionPages", "extensionServiceWorkers"} {
		delete(clean, key)
	}
	if s, ok := result["structuredContent"].(map[string]any); ok {
		fields := maps.Clone(s)
		for _, key := range []string{"pages", "extensionPages", "extensionServiceWorkers"} {
			delete(fields, key)
		}
		clean["structuredContent"] = fields
	}
	// All text is redundant with structuredContent in the pinned engine, and
	// may contain a page catalog even if its structured representation is absent.
	delete(clean, "content")
	if content, ok := result["content"].([]any); ok {
		var retained []any
		for _, item := range content {
			if block, ok := item.(map[string]any); ok && block["type"] != "text" {
				retained = append(retained, block)
			}
		}
		if len(retained) > 0 {
			clean["content"] = retained
		}
	}
	return clean
}
