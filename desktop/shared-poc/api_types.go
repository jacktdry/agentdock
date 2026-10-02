package main

import "github.com/uvwt/agentdock/internal/desktopruntime"

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type RuntimeStatusResult struct {
	RuntimeRoot string                       `json:"runtimeRoot"`
	Status      desktopruntime.ServiceStatus `json:"status"`
	Error       *APIError                    `json:"error,omitempty"`
}

type RuntimeActionResult struct {
	Action    string    `json:"action"`
	Completed bool      `json:"completed"`
	Error     *APIError `json:"error,omitempty"`
}

type Preferences struct {
	BatchIntervalMS int `json:"batchIntervalMs"`
	WindowWidth     int `json:"windowWidth"`
	WindowHeight    int `json:"windowHeight"`
}

type PreferencesResult struct {
	Preferences Preferences `json:"preferences"`
	Error       *APIError   `json:"error,omitempty"`
}

type SavePreferencesResult struct {
	Saved bool      `json:"saved"`
	Error *APIError `json:"error,omitempty"`
}

type SyntheticEvent struct {
	Sequence int64  `json:"sequence"`
	Time     string `json:"time"`
}

type EventBatch struct {
	Events           []SyntheticEvent `json:"events"`
	Produced         int64            `json:"produced"`
	Delivered        int64            `json:"delivered"`
	Dropped          int64            `json:"dropped"`
	QueueDropped     int64            `json:"queueDropped"`
	TransportDropped int64            `json:"transportDropped"`
	QueueDepth       int              `json:"queueDepth"`
	QueueCapacity    int              `json:"queueCapacity"`
	BatchIntervalMS  int              `json:"batchIntervalMs"`
}

type EventSourceStatus struct {
	Running          bool  `json:"running"`
	RateHz           int   `json:"rateHz"`
	Produced         int64 `json:"produced"`
	Delivered        int64 `json:"delivered"`
	Dropped          int64 `json:"dropped"`
	QueueDropped     int64 `json:"queueDropped"`
	TransportDropped int64 `json:"transportDropped"`
	QueueDepth       int   `json:"queueDepth"`
	QueueCapacity    int   `json:"queueCapacity"`
}

type EventControlResult struct {
	Status EventSourceStatus `json:"status"`
	Error  *APIError         `json:"error,omitempty"`
}

func apiError(code string, err error) *APIError {
	if err == nil {
		return nil
	}
	return &APIError{Code: code, Message: err.Error()}
}
