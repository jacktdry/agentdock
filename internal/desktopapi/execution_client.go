package desktopapi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/uvwt/agentdock/internal/desktopruntime"
	"github.com/uvwt/agentdock/internal/execution"
)

const maxExecutionResponseBytes = 2 << 20

type ExecutionStreamMessage struct {
	Kind string
	ID   string
	Page execution.Page
}

type ExecutionClient struct {
	runtimeRoot string
	rootError   error
	httpClient  *http.Client
	streamHTTP  *http.Client
}

func NewExecutionClient(root string) *ExecutionClient {
	root, err := resolveRuntimeRoot(root)
	transport := &http.Transport{
		Proxy:                 nil,
		DisableCompression:    false,
		ForceAttemptHTTP2:     false,
		ResponseHeaderTimeout: 5 * time.Second,
	}
	return &ExecutionClient{
		runtimeRoot: root,
		rootError:   err,
		httpClient:  &http.Client{Transport: transport.Clone(), Timeout: 8 * time.Second},
		streamHTTP:  &http.Client{Transport: transport},
	}
}

func (c *ExecutionClient) Snapshot(ctx context.Context) (execution.Snapshot, error) {
	var response struct {
		OK       bool
		Snapshot execution.Snapshot
	}
	if err := c.getJSON(ctx, "/internal/runtime/execution", nil, &response); err != nil {
		return execution.Snapshot{}, err
	}
	if !response.OK || response.Snapshot.Epoch == "" {
		return execution.Snapshot{}, errors.New("invalid execution snapshot")
	}
	return response.Snapshot, nil
}

func (c *ExecutionClient) Replay(ctx context.Context, after uint64, limit int) (execution.Page, error) {
	if limit < 1 || limit > execution.MaxPageEvents {
		limit = 200
	}
	query := url.Values{
		"after": {strconv.FormatUint(after, 10)},
		"limit": {strconv.Itoa(limit)},
	}
	var response struct {
		OK   bool
		Page execution.Page
	}
	if err := c.getJSON(ctx, "/internal/runtime/activity", query, &response); err != nil {
		return execution.Page{}, err
	}
	if !response.OK || response.Page.Epoch == "" {
		return execution.Page{}, errors.New("invalid execution replay page")
	}
	return response.Page, nil
}

func (c *ExecutionClient) EnqueueInsertion(ctx context.Context, callID, text string) (execution.Insertion, error) {
	if !utf8.ValidString(text) || strings.TrimSpace(text) == "" || len(strings.TrimSpace(text)) > execution.MaxInsertionTextBytes {
		return execution.Insertion{}, execution.ErrInsertionInvalid
	}
	var response struct {
		OK        bool
		ACK       bool
		Insertion execution.Insertion
	}
	body := map[string]any{"action": "enqueue", "call_id": strings.TrimSpace(callID), "text": text}
	if err := c.postJSON(ctx, "/internal/runtime/insertions", body, &response); err != nil {
		return execution.Insertion{}, err
	}
	if !response.OK || !response.ACK || response.Insertion.ID == "" || response.Insertion.TargetCallID != strings.TrimSpace(callID) || response.Insertion.Status != execution.InsertionAccepted {
		return execution.Insertion{}, errors.New("invalid insertion acknowledgement")
	}
	return response.Insertion, nil
}

func (c *ExecutionClient) CancelInsertion(ctx context.Context, insertionID string) (execution.Insertion, error) {
	var response struct {
		OK        bool
		Insertion execution.Insertion
	}
	body := map[string]any{"action": "cancel", "insertion_id": strings.TrimSpace(insertionID)}
	if err := c.postJSON(ctx, "/internal/runtime/insertions", body, &response); err != nil {
		return execution.Insertion{}, err
	}
	if !response.OK || response.Insertion.ID == "" {
		return execution.Insertion{}, errors.New("invalid insertion cancellation response")
	}
	return response.Insertion, nil
}

func (c *ExecutionClient) Stream(ctx context.Context, epoch string, after uint64, onMessage func(ExecutionStreamMessage) error) error {
	if onMessage == nil {
		return errors.New("execution stream callback is required")
	}
	connection, err := c.connection()
	if err != nil {
		return err
	}
	query := url.Values{"after": {strconv.FormatUint(after, 10)}}
	if epoch = strings.TrimSpace(epoch); epoch != "" {
		query.Set("epoch", epoch)
	}
	endpoint := connection.Endpoint() + "/internal/runtime/activity/stream?" + query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "text/event-stream")
	if token := connection.AuthToken(); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := c.streamHTTP.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("Core activity stream returned HTTP %d", response.StatusCode)
	}
	if !strings.HasPrefix(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
		return errors.New("Core activity stream returned an invalid content type")
	}
	if err := onMessage(ExecutionStreamMessage{Kind: "connected"}); err != nil {
		return err
	}

	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 4096), maxExecutionResponseBytes)
	eventName := ""
	eventID := ""
	data := ""
	dispatch := func() error {
		if data == "" {
			eventName, eventID = "", ""
			return nil
		}
		if eventName != "activity" && eventName != "reset" {
			eventName, eventID, data = "", "", ""
			return nil
		}
		var page execution.Page
		if err := json.Unmarshal([]byte(data), &page); err != nil {
			return errors.New("invalid Core activity stream payload")
		}
		if page.Epoch == "" {
			return errors.New("Core activity stream payload is missing epoch")
		}
		message := ExecutionStreamMessage{Kind: eventName, ID: eventID, Page: page}
		eventName, eventID, data = "", "", ""
		return onMessage(message)
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := dispatch(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		switch {
		case strings.HasPrefix(line, "event:"):
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "id:"):
			eventID = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
		case strings.HasPrefix(line, "data:"):
			part := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if len(data)+len(part)+1 > maxExecutionResponseBytes {
				return errors.New("Core activity stream payload is too large")
			}
			if data == "" {
				data = part
			} else {
				data += "\n" + part
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return ctx.Err()
}

func (c *ExecutionClient) getJSON(ctx context.Context, path string, query url.Values, target any) error {
	connection, err := c.connection()
	if err != nil {
		return err
	}
	endpoint := connection.Endpoint() + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	return c.doJSON(request, connection.AuthToken(), target)
}

func (c *ExecutionClient) postJSON(ctx context.Context, path string, body any, target any) error {
	connection, err := c.connection()
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return err
	}
	if len(encoded) > execution.MaxInsertionRequestBytes {
		return errors.New("Core execution request is too large")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, connection.Endpoint()+path, strings.NewReader(string(encoded)))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	return c.doJSON(request, connection.AuthToken(), target)
}

func (c *ExecutionClient) doJSON(request *http.Request, token string, target any) error {
	request.Header.Set("Accept", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("Core execution API returned HTTP %d", response.StatusCode)
	}
	reader := io.LimitReader(response.Body, maxExecutionResponseBytes+1)
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	if len(data) > maxExecutionResponseBytes {
		return errors.New("Core execution response is too large")
	}
	if err := json.Unmarshal(data, target); err != nil {
		return errors.New("Core execution response is invalid")
	}
	return nil
}

func (c *ExecutionClient) connection() (desktopruntime.CoreConnection, error) {
	if c == nil {
		return desktopruntime.CoreConnection{}, errors.New("execution client is unavailable")
	}
	if c.rootError != nil {
		return desktopruntime.CoreConnection{}, c.rootError
	}
	return desktopruntime.ReadCoreConnection(c.runtimeRoot)
}
