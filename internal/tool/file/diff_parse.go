package file

import (
	"errors"
	"strconv"
	"strings"

	"github.com/uvwt/agentdock/internal/textutil"
)

func unifiedPatchPaths(diffText string) ([]string, error) {
	seen := make(map[string]struct{})
	paths := make([]string, 0)
	add := func(raw string) error {
		path, err := parseUnifiedPatchPath(raw)
		if err != nil {
			return err
		}
		if path == "" || path == "/dev/null" {
			return nil
		}
		path = strings.TrimPrefix(strings.TrimPrefix(path, "a/"), "b/")
		if path == "" {
			return errors.New("unified patch path is empty")
		}
		if _, exists := seen[path]; exists {
			return nil
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
		return nil
	}

	for _, line := range strings.Split(diffText, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			tokens, err := splitGitPatchPathTokens(strings.TrimPrefix(line, "diff --git "))
			if err != nil || len(tokens) != 2 {
				return nil, errors.New("invalid diff --git path header")
			}
			for _, token := range tokens {
				if err := add(token); err != nil {
					return nil, err
				}
			}
		case strings.HasPrefix(line, "--- "):
			if err := add(strings.TrimPrefix(line, "--- ")); err != nil {
				return nil, err
			}
		case strings.HasPrefix(line, "+++ "):
			if err := add(strings.TrimPrefix(line, "+++ ")); err != nil {
				return nil, err
			}
		case strings.HasPrefix(line, "rename from "):
			if err := add(strings.TrimPrefix(line, "rename from ")); err != nil {
				return nil, err
			}
		case strings.HasPrefix(line, "rename to "):
			if err := add(strings.TrimPrefix(line, "rename to ")); err != nil {
				return nil, err
			}
		case strings.HasPrefix(line, "copy from "):
			if err := add(strings.TrimPrefix(line, "copy from ")); err != nil {
				return nil, err
			}
		case strings.HasPrefix(line, "copy to "):
			if err := add(strings.TrimPrefix(line, "copy to ")); err != nil {
				return nil, err
			}
		}
	}
	if len(paths) == 0 {
		return nil, errors.New("unified patch contains no file paths")
	}
	return paths, nil
}

func parseUnifiedPatchPath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("unified patch path is empty")
	}
	if strings.HasPrefix(raw, "\"") {
		tokens, err := splitGitPatchPathTokens(raw)
		if err != nil || len(tokens) == 0 {
			return "", errors.New("invalid quoted unified patch path")
		}
		return tokens[0], nil
	}
	if tab := strings.IndexByte(raw, '\t'); tab >= 0 {
		raw = raw[:tab]
	}
	return strings.TrimSpace(raw), nil
}

func splitGitPatchPathTokens(raw string) ([]string, error) {
	result := make([]string, 0, 2)
	for {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return result, nil
		}
		if raw[0] != '"' {
			end := strings.IndexAny(raw, " \t")
			if end < 0 {
				result = append(result, raw)
				return result, nil
			}
			result = append(result, raw[:end])
			raw = raw[end:]
			continue
		}
		end := 1
		escaped := false
		for ; end < len(raw); end++ {
			char := raw[end]
			if escaped {
				escaped = false
				continue
			}
			if char == '\\' {
				escaped = true
				continue
			}
			if char == '"' {
				break
			}
		}
		if end >= len(raw) || raw[end] != '"' {
			return nil, errors.New("unterminated quoted unified patch path")
		}
		decoded, err := strconv.Unquote(raw[:end+1])
		if err != nil {
			return nil, err
		}
		result = append(result, decoded)
		raw = raw[end+1:]
	}
}

type gitDiffFile struct {
	Path   string `json:"path"`
	Status string `json:"status"`
	Binary bool   `json:"binary"`
}

// parseDiffFiles 解析统一 diff 的文件级元数据，既用于 Git diff，也用于 file_edit patch 预览。
func parseDiffFiles(diffText string) []gitDiffFile {
	files := make([]gitDiffFile, 0)
	current := -1
	for _, line := range strings.Split(diffText, "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			parts := strings.Fields(line)
			path := ""
			if len(parts) >= 4 {
				path = strings.TrimPrefix(parts[3], "b/")
			}
			files = append(files, gitDiffFile{Path: path, Status: "modified"})
			current = len(files) - 1
			continue
		}
		if current < 0 {
			continue
		}
		if strings.HasPrefix(line, "new file mode") {
			files[current].Status = "added"
		}
		if strings.HasPrefix(line, "deleted file mode") {
			files[current].Status = "deleted"
		}
		if strings.HasPrefix(line, "Binary files") {
			files[current].Binary = true
		}
	}
	return files
}

func truncateBytes(data []byte, maxBytes int) (string, bool) {
	truncated := textutil.SafeTruncateBytes(data, maxBytes)
	return truncated.Text, truncated.Truncated
}
