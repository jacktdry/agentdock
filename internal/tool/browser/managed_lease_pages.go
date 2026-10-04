package browser

import (
	"errors"
	"math"
	"strconv"
)

// Worker transport JSON-decodes numeric IDs as float64. Fail closed on malformed
// structured content; text and selected-page state never supply a target.
func isolatedPageID(result map[string]any, isolation string) (float64, string, error) {
	structured, ok := result["structuredContent"].(map[string]any)
	if !ok {
		return 0, "", errors.New("missing structuredContent")
	}
	pages, ok := structured["pages"].([]any)
	if !ok {
		return 0, "", errors.New("pages must be an array")
	}
	if isolation == "" || structured["reconnected"] == true {
		return 0, "", errors.New("isolated context identity unavailable")
	}
	matches := 0
	var id float64
	seen := make(map[float64]bool, len(pages))
	for _, entry := range pages {
		page, ok := entry.(map[string]any)
		if !ok {
			return 0, "", errors.New("invalid page entry")
		}
		pageID, ok := page["id"].(float64)
		if !ok || math.IsNaN(pageID) || math.IsInf(pageID, 0) || pageID < 0 || math.Trunc(pageID) != pageID || pageID > 9007199254740991 {
			return 0, "", errors.New("invalid numeric page id")
		}
		if seen[pageID] {
			return 0, "", errors.New("duplicate page id")
		}
		seen[pageID] = true
		contextName, present := page["isolatedContext"]
		if present {
			if name, ok := contextName.(string); !ok || name == "" {
				return 0, "", errors.New("invalid isolated context name")
			}
		}
		if contextName == isolation {
			matches++
			id = pageID
		}
	}
	if matches != 1 {
		return 0, "", errors.New("expected exactly one isolated page")
	}
	return id, strconv.FormatFloat(id, 'f', 0, 64), nil
}
