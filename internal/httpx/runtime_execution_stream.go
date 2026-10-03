package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/uvwt/agentdock/internal/auth"
	"github.com/uvwt/agentdock/internal/config"
	"github.com/uvwt/agentdock/internal/execution"
	"github.com/uvwt/agentdock/internal/runtimeapi"
)

const (
	runtimeActivityStreamLimit       = 100
	runtimeActivityStreamClients     = 32
	runtimeActivityHeartbeatInterval = 15 * time.Second
	runtimeActivityWriteTimeout      = 5 * time.Second
)

func runtimeActivityStreamHandler(runtime runtimeapi.Runtime, cfg config.Config, oauthStore *auth.OAuthStore) http.HandlerFunc {
	slots := make(chan struct{}, runtimeActivityStreamClients)
	authorizer := auth.Bearer{Token: cfg.AuthToken}
	authRequired := cfg.AuthRequired()

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeRuntimeAPIError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		if !isDirectLoopbackRequest(r) {
			writeRuntimeAPIError(w, http.StatusForbidden, "LOCAL_ACCESS_REQUIRED", "runtime activity stream requires direct local access")
			return
		}
		if authRequired {
			staticOK := cfg.AuthToken != "" && authorizer.Authorized(r)
			oauthOK := authorizedOAuth(r, cfg, oauthStore)
			if !staticOK && !oauthOK {
				setBearerChallenge(w, cfg, r, strings.TrimSpace(r.Header.Get("Authorization")) != "")
				writeRuntimeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
				return
			}
		}
		executionRuntime, ok := runtime.(runtimeapi.ExecutionRuntime)
		if !ok {
			writeRuntimeAPIError(w, http.StatusNotFound, "EXECUTION_UNSUPPORTED", "runtime does not support execution activity")
			return
		}
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			writeRuntimeAPIError(w, http.StatusServiceUnavailable, "ACTIVITY_STREAM_LIMIT", "too many activity stream clients")
			return
		}

		after, err := activityStreamCursor(r)
		if err != nil {
			writeRuntimeAPIError(w, http.StatusBadRequest, "INVALID_ACTIVITY_CURSOR", "activity cursor must be an unsigned decimal sequence")
			return
		}
		expectedEpoch := strings.TrimSpace(r.URL.Query().Get("epoch"))
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeRuntimeAPIError(w, http.StatusInternalServerError, "STREAM_UNSUPPORTED", "streaming response is unavailable")
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache, no-store")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		controller := http.NewResponseController(w)
		for {
			result, pageErr := executionRuntime.RuntimeActivity(after, runtimeActivityStreamLimit)
			if pageErr != nil {
				return
			}
			page, ok := result["page"].(execution.Page)
			if !ok {
				return
			}
			latest, parseErr := strconv.ParseUint(page.LatestSequence, 10, 64)
			if parseErr != nil {
				return
			}
			if (expectedEpoch != "" && page.Epoch != expectedEpoch) || after > latest {
				_ = writeRuntimeSSE(controller, w, flusher, "", "reset", page)
				return
			}
			if page.Gap {
				if err := writeRuntimeSSE(controller, w, flusher, "", "reset", page); err != nil {
					return
				}
				latest, _ := strconv.ParseUint(page.LatestSequence, 10, 64)
				after = latest
				continue
			}
			if len(page.Events) > 0 {
				last := page.Events[len(page.Events)-1].Sequence
				if err := writeRuntimeSSE(controller, w, flusher, last, "activity", page); err != nil {
					return
				}
				next, parseErr := strconv.ParseUint(last, 10, 64)
				if parseErr != nil {
					return
				}
				after = next
				if page.HasMore {
					continue
				}
			}

			waitCtx, cancel := context.WithTimeout(r.Context(), runtimeActivityHeartbeatInterval)
			waitErr := executionRuntime.RuntimeActivityWait(waitCtx, after)
			cancel()
			switch {
			case waitErr == nil:
				continue
			case errors.Is(waitErr, context.DeadlineExceeded):
				if err := writeRuntimeSSEHeartbeat(controller, w, flusher); err != nil {
					return
				}
			case errors.Is(waitErr, context.Canceled):
				return
			default:
				return
			}
		}
	}
}

func activityStreamCursor(r *http.Request) (uint64, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("after"))
	if raw == "" {
		raw = strings.TrimSpace(r.Header.Get("Last-Event-ID"))
	}
	if raw == "" {
		return 0, nil
	}
	return strconv.ParseUint(raw, 10, 64)
}

func writeRuntimeSSE(controller *http.ResponseController, w http.ResponseWriter, flusher http.Flusher, id, event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if err := controller.SetWriteDeadline(time.Now().Add(runtimeActivityWriteTimeout)); err != nil {
		return err
	}
	if id != "" {
		if _, err := fmt.Fprintf(w, "id: %s\n", id); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func writeRuntimeSSEHeartbeat(controller *http.ResponseController, w http.ResponseWriter, flusher http.Flusher) error {
	if err := controller.SetWriteDeadline(time.Now().Add(runtimeActivityWriteTimeout)); err != nil {
		return err
	}
	if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}
