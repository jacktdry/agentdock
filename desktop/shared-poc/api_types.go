package main

import "github.com/uvwt/agentdock/internal/desktopapi"

type Preferences struct {
	BatchIntervalMS int `json:"batchIntervalMs"`
	WindowWidth     int `json:"windowWidth"`
	WindowHeight    int `json:"windowHeight"`
}

type PreferencesResult struct {
	Preferences Preferences          `json:"preferences"`
	Error       *desktopapi.APIError `json:"error,omitempty"`
}

type SavePreferencesResult struct {
	Saved bool                 `json:"saved"`
	Error *desktopapi.APIError `json:"error,omitempty"`
}

type ActivityProbeStatus struct {
	Running               bool   `json:"running"`
	RateHz                int    `json:"rateHz"`
	Epoch                 string `json:"epoch"`
	LatestSequence        string `json:"latestSequence"`
	PublishedTotal        string `json:"publishedTotal"`
	DeliveredTotal        string `json:"deliveredTotal"`
	DroppedTotal          string `json:"droppedTotal"`
	SourceDroppedTotal    string `json:"sourceDroppedTotal"`
	TransportDroppedTotal string `json:"transportDroppedTotal"`
	QueueDepth            int    `json:"queueDepth"`
	QueueCapacity         int    `json:"queueCapacity"`
}

type ActivityProbeControlResult struct {
	Status ActivityProbeStatus  `json:"status"`
	Error  *desktopapi.APIError `json:"error,omitempty"`
}
