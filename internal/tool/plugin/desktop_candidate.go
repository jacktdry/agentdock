package plugin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	pluginruntime "github.com/uvwt/agentdock/internal/plugin"
)

const (
	desktopCandidateTTL  = 15 * time.Minute
	maxDesktopCandidates = 64
)

type DesktopCandidateSkill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type DesktopCandidateMCP struct {
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	Transport        string   `json:"transport"`
	Endpoint         string   `json:"endpoint,omitempty"`
	Command          string   `json:"command,omitempty"`
	EnvironmentNames []string `json:"environment_names"`
	HeaderNames      []string `json:"header_names"`
}

type DesktopCandidateProvenance struct {
	Origin   string `json:"origin,omitempty"`
	Ref      string `json:"ref,omitempty"`
	Revision string `json:"revision,omitempty"`
	Subdir   string `json:"subdir,omitempty"`
}

type DesktopCandidateReview struct {
	Valid              bool                        `json:"valid"`
	Name               string                      `json:"name"`
	Version            string                      `json:"version"`
	Description        string                      `json:"description"`
	Format             string                      `json:"format"`
	PackageFingerprint string                      `json:"package_fingerprint"`
	Provenance         *DesktopCandidateProvenance `json:"provenance,omitempty"`
	Skills             []DesktopCandidateSkill     `json:"skills"`
	MCP                []DesktopCandidateMCP       `json:"mcp"`
	Executables        []string                    `json:"executables"`
	Warnings           []string                    `json:"warnings"`
	Issues             []string                    `json:"issues"`
}

type DesktopCandidateView struct {
	CandidateID string                  `json:"candidate_id"`
	Kind        string                  `json:"kind"`
	TargetName  string                  `json:"target_name,omitempty"`
	ExpiresAt   string                  `json:"expires_at"`
	Review      DesktopCandidateReview  `json:"review"`
	Current     *DesktopCandidateReview `json:"current,omitempty"`
}

type desktopCandidateRecord struct {
	id               string
	kind             string
	targetName       string
	targetGeneration string
	expiresAt        time.Time
	prepared         *pluginruntime.PreparedCandidate
	view             DesktopCandidateView
}

func newDesktopCandidateID() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func validDesktopCandidateID(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func safeCandidateText(value string, max int, replacements ...string) string {
	value = strings.TrimSpace(value)
	for _, raw := range replacements {
		if raw != "" {
			value = strings.ReplaceAll(value, raw, "[redacted]")
		}
	}
	value = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, value)
	value = redactAbsolutePathTokens(value)
	if len(value) > max {
		value = value[:max]
	}
	return value
}

func redactAbsolutePathTokens(value string) string {
	parts := strings.FieldsFunc(value, func(r rune) bool { return unicode.IsSpace(r) })
	if len(parts) == 0 {
		return value
	}
	for index, part := range parts {
		trimmed := strings.Trim(part, "()[]{}<>,;:'\"")
		windowsAbs := len(trimmed) >= 3 && ((trimmed[0] >= 'A' && trimmed[0] <= 'Z') || (trimmed[0] >= 'a' && trimmed[0] <= 'z')) &&
			trimmed[1] == ':' && (trimmed[2] == '\\' || trimmed[2] == '/')
		uncAbs := strings.HasPrefix(trimmed, `\\`)
		unixAbs := strings.HasPrefix(trimmed, "/") && !strings.HasPrefix(trimmed, "//")
		if windowsAbs || uncAbs || unixAbs {
			parts[index] = "[redacted-path]"
		}
	}
	return strings.Join(parts, " ")
}

func safeCandidateOpaqueLabel(value, source, home string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, "://") || strings.HasPrefix(value, "/") || strings.HasPrefix(value, `\\`) ||
		(len(value) >= 3 && value[1] == ':' && (value[2] == '\\' || value[2] == '/')) {
		return ""
	}
	return safeCandidateText(value, 256, source, home)
}

func safeCandidateOrigin(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.User != nil || parsed.Host == "" {
		return ""
	}
	switch parsed.Scheme {
	case "http", "https":
		return parsed.Scheme + "://" + parsed.Host
	default:
		return ""
	}
}

func safeCandidateRelativePath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || filepath.IsAbs(raw) {
		return ""
	}
	clean := filepath.Clean(raw)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return ""
	}
	return safeCandidateText(filepath.ToSlash(clean), 256)
}

func safeCandidateReview(review pluginruntime.Review, source, home string) DesktopCandidateReview {
	out := DesktopCandidateReview{
		Valid: review.Valid, Name: safeCandidateText(review.Name, 128), Version: safeCandidateText(review.Version, 128),
		Description: safeCandidateText(review.Description, 512, source, home), Format: safeCandidateText(review.Format, 32),
		PackageFingerprint: safeCandidateText(review.PackageDigest, 128),
		Skills:             []DesktopCandidateSkill{}, MCP: []DesktopCandidateMCP{}, Executables: []string{}, Warnings: []string{}, Issues: []string{},
	}
	if review.Provenance != nil {
		out.Provenance = &DesktopCandidateProvenance{
			Origin:   safeCandidateOrigin(review.Provenance.Origin),
			Ref:      safeCandidateOpaqueLabel(review.Provenance.Ref, source, home),
			Revision: safeCandidateOpaqueLabel(review.Provenance.Revision, source, home),
			Subdir:   safeCandidateRelativePath(review.Provenance.Subdir),
		}
	}
	for _, skill := range review.Skills {
		out.Skills = append(out.Skills, DesktopCandidateSkill{
			Name: safeCandidateText(skill.Name, 128), Description: safeCandidateText(skill.Description, 512, source, home),
		})
	}
	for _, component := range review.MCP {
		command := ""
		if strings.TrimSpace(component.Command) != "" {
			command = safeCandidateText(filepath.Base(component.Command), 128, source, home)
		}
		envNames := append([]string(nil), component.EnvironmentNames...)
		headerNames := append([]string(nil), component.HeaderNames...)
		sort.Strings(envNames)
		sort.Strings(headerNames)
		out.MCP = append(out.MCP, DesktopCandidateMCP{
			Name: safeCandidateText(component.Name, 128), Description: safeCandidateText(component.Description, 512, source, home),
			Transport: safeCandidateText(component.Transport, 32), Endpoint: safeCandidateOrigin(component.URL), Command: command,
			EnvironmentNames: envNames, HeaderNames: headerNames,
		})
	}
	for _, executable := range review.Executables {
		if safe := safeCandidateRelativePath(executable); safe != "" {
			out.Executables = append(out.Executables, safe)
		}
	}
	for _, warning := range review.Warnings {
		if safe := safeCandidateText(warning, 512, source, home); safe != "" {
			out.Warnings = append(out.Warnings, safe)
		}
	}
	for _, issue := range review.Issues {
		if safe := safeCandidateText(issue, 512, source, home); safe != "" {
			out.Issues = append(out.Issues, safe)
		}
	}
	return out
}

func (s *Service) cleanupDesktopCandidatesLocked(now time.Time) {
	for id, candidate := range s.desktopCandidates {
		if candidate == nil || !candidate.expiresAt.After(now) {
			if candidate != nil && candidate.prepared != nil {
				candidate.prepared.Close()
			}
			delete(s.desktopCandidates, id)
		}
	}
}

func (s *Service) PrepareDesktopCandidate(ctx context.Context, source, kind, targetName, targetGeneration string) (DesktopCandidateView, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind != "install" && kind != "update" {
		return DesktopCandidateView{}, DesktopError("PLUGIN_CANDIDATE_INVALID", "validation")
	}
	prepared, err := s.manager.PrepareCandidate(source)
	if err != nil {
		return DesktopCandidateView{}, DesktopError("PLUGIN_CANDIDATE_INVALID", "validation")
	}
	review, err := prepared.Review()
	if err != nil {
		prepared.Close()
		return DesktopCandidateView{}, DesktopError("PLUGIN_CANDIDATE_INVALID", "validation")
	}
	if !review.Valid {
		prepared.Close()
		return DesktopCandidateView{}, DesktopError("PLUGIN_CANDIDATE_INVALID", "validation")
	}

	release, err := s.manager.Store().AcquireManagement(ctx)
	if err != nil {
		prepared.Close()
		return DesktopCandidateView{}, DesktopError("PLUGIN_CORE_UNAVAILABLE", "unavailable")
	}
	defer release()
	registry, err := s.manager.Store().DesktopRegistrySnapshot()
	if err != nil {
		prepared.Close()
		return DesktopCandidateView{}, DesktopError("PLUGIN_REGISTRY_READ_FAILED", "unavailable")
	}

	var current *DesktopCandidateReview
	if kind == "install" {
		for _, state := range registry.States {
			if state.Name == review.Name {
				prepared.Close()
				return DesktopCandidateView{}, DesktopError("PLUGIN_ALREADY_INSTALLED", "conflict")
			}
		}
		targetName, targetGeneration = "", ""
	} else {
		targetName = strings.TrimSpace(targetName)
		if pluginruntime.ValidateName(targetName) != nil || review.Name != targetName || len(targetGeneration) != 64 {
			prepared.Close()
			return DesktopCandidateView{}, DesktopError("PLUGIN_CANDIDATE_TARGET_MISMATCH", "conflict")
		}
		for _, recovery := range registry.Recoveries {
			if recovery.Name == targetName {
				prepared.Close()
				return DesktopCandidateView{}, DesktopError("PLUGIN_RECOVERY_REQUIRED", "conflict")
			}
		}
		found := false
		for _, state := range registry.States {
			if state.Name != targetName {
				continue
			}
			found = true
			if pluginruntime.DesktopGeneration(state) != targetGeneration {
				prepared.Close()
				return DesktopCandidateView{}, DesktopError("PLUGIN_GENERATION_CONFLICT", "conflict")
			}
			installed, inspectErr := s.manager.Inspect(targetName)
			if inspectErr != nil {
				prepared.Close()
				return DesktopCandidateView{}, DesktopError("PLUGIN_PACKAGE_UNAVAILABLE", "unavailable")
			}
			currentReview := s.manager.Validate(installed.Root)
			if !currentReview.Valid || currentReview.PackageDigest != installed.PackageDigest {
				prepared.Close()
				return DesktopCandidateView{}, DesktopError("PLUGIN_PACKAGE_UNAVAILABLE", "unavailable")
			}
			safe := safeCandidateReview(currentReview, "", s.manager.Store().Home())
			current = &safe
			break
		}
		if !found {
			prepared.Close()
			return DesktopCandidateView{}, DesktopError("PLUGIN_NOT_FOUND", "not_found")
		}
	}

	id, err := newDesktopCandidateID()
	if err != nil {
		prepared.Close()
		return DesktopCandidateView{}, DesktopError("PLUGIN_CANDIDATE_UNAVAILABLE", "unavailable")
	}
	now := time.Now().UTC()
	view := DesktopCandidateView{
		CandidateID: id, Kind: kind, TargetName: targetName, ExpiresAt: now.Add(desktopCandidateTTL).Format(time.RFC3339Nano),
		Review: safeCandidateReview(review, source, s.manager.Store().Home()), Current: current,
	}
	record := &desktopCandidateRecord{
		id: id, kind: kind, targetName: targetName, targetGeneration: targetGeneration,
		expiresAt: now.Add(desktopCandidateTTL), prepared: prepared, view: view,
	}
	s.desktopCandidateMu.Lock()
	s.cleanupDesktopCandidatesLocked(now)
	if len(s.desktopCandidates) >= maxDesktopCandidates {
		s.desktopCandidateMu.Unlock()
		prepared.Close()
		return DesktopCandidateView{}, DesktopError("PLUGIN_CANDIDATE_LIMIT", "capacity")
	}
	s.desktopCandidates[id] = record
	s.desktopCandidateMu.Unlock()
	return view, nil
}

func (s *Service) DiscardDesktopCandidate(candidateID string) error {
	if !validDesktopCandidateID(candidateID) {
		return DesktopError("PLUGIN_CANDIDATE_INVALID", "validation")
	}
	now := time.Now().UTC()
	s.desktopCandidateMu.Lock()
	s.cleanupDesktopCandidatesLocked(now)
	record := s.desktopCandidates[candidateID]
	delete(s.desktopCandidates, candidateID)
	s.desktopCandidateMu.Unlock()
	if record == nil {
		return DesktopError("PLUGIN_CANDIDATE_UNAVAILABLE", "not_found")
	}
	record.prepared.Close()
	return nil
}

func (s *Service) takeDesktopCandidate(candidateID, kind, targetName, targetGeneration string) (*pluginruntime.PreparedCandidate, error) {
	if !validDesktopCandidateID(candidateID) {
		return nil, DesktopError("PLUGIN_CANDIDATE_INVALID", "validation")
	}
	now := time.Now().UTC()
	s.desktopCandidateMu.Lock()
	s.cleanupDesktopCandidatesLocked(now)
	record := s.desktopCandidates[candidateID]
	if record == nil {
		s.desktopCandidateMu.Unlock()
		return nil, DesktopError("PLUGIN_CANDIDATE_UNAVAILABLE", "not_found")
	}
	targetName = strings.TrimSpace(targetName)
	targetMismatch := record.kind != kind
	if kind == "install" {
		targetMismatch = targetMismatch || record.view.Review.Name != targetName
	} else {
		targetMismatch = targetMismatch || record.targetName != targetName || record.targetGeneration != targetGeneration
	}
	if targetMismatch {
		s.desktopCandidateMu.Unlock()
		return nil, DesktopError("PLUGIN_CANDIDATE_TARGET_MISMATCH", "conflict")
	}
	delete(s.desktopCandidates, candidateID)
	s.desktopCandidateMu.Unlock()
	return record.prepared, nil
}
