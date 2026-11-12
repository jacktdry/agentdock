package acp

import (
	"strings"
	"time"
)

type SessionLifecyclePolicy string

const (
	LifecyclePersistent  SessionLifecyclePolicy = "persistent"
	LifecycleEphemeral   SessionLifecyclePolicy = "ephemeral"
	LifecycleIdleManaged SessionLifecyclePolicy = "idle-managed"

	DefaultIdleCloseAfter = 30 * time.Minute
	MinIdleCloseAfter     = time.Minute
	MaxIdleCloseAfter     = 7 * 24 * time.Hour
)

type SessionLifecycleOptions struct {
	Policy         SessionLifecyclePolicy
	IdleCloseAfter time.Duration
}

func NormalizeSessionLifecycleOptions(options SessionLifecycleOptions) (SessionLifecycleOptions, error) {
	options.Policy = SessionLifecyclePolicy(strings.TrimSpace(string(options.Policy)))
	if options.Policy == "" {
		options.Policy = LifecyclePersistent
	}
	switch options.Policy {
	case LifecyclePersistent, LifecycleEphemeral:
		if options.IdleCloseAfter != 0 {
			return SessionLifecycleOptions{}, newError("ACP_SESSION_LIFECYCLE_INVALID", "idle_close_after_ms is only valid for idle-managed sessions", false, map[string]any{"lifecycle_policy": options.Policy}, nil)
		}
	case LifecycleIdleManaged:
		if options.IdleCloseAfter == 0 {
			options.IdleCloseAfter = DefaultIdleCloseAfter
		}
		if options.IdleCloseAfter < MinIdleCloseAfter || options.IdleCloseAfter > MaxIdleCloseAfter {
			return SessionLifecycleOptions{}, newError("ACP_SESSION_LIFECYCLE_INVALID", "idle-managed close TTL is outside the supported range", false, map[string]any{
				"minimum_ms": MinIdleCloseAfter.Milliseconds(), "maximum_ms": MaxIdleCloseAfter.Milliseconds(), "idle_close_after_ms": options.IdleCloseAfter.Milliseconds(),
			}, nil)
		}
	default:
		return SessionLifecycleOptions{}, newError("ACP_SESSION_LIFECYCLE_INVALID", "unsupported ACP session lifecycle policy", false, map[string]any{"lifecycle_policy": options.Policy}, nil)
	}
	return options, nil
}

func normalizeSessionRecord(record SessionRecord) SessionRecord {
	if record.LifecyclePolicy == "" {
		record.LifecyclePolicy = LifecyclePersistent
	}
	if record.LastActiveAt.IsZero() {
		switch {
		case !record.UpdatedAt.IsZero():
			record.LastActiveAt = record.UpdatedAt
		case !record.CreatedAt.IsZero():
			record.LastActiveAt = record.CreatedAt
		}
	}
	if record.LifecyclePolicy == LifecycleIdleManaged && record.IdleCloseAfterMS == 0 {
		record.IdleCloseAfterMS = DefaultIdleCloseAfter.Milliseconds()
	}
	return record
}

func applyLifecycleOptions(record *SessionRecord, options SessionLifecycleOptions) {
	if record == nil {
		return
	}
	record.LifecyclePolicy = options.Policy
	if options.Policy == LifecycleIdleManaged {
		record.IdleCloseAfterMS = options.IdleCloseAfter.Milliseconds()
	} else {
		record.IdleCloseAfterMS = 0
	}
}
