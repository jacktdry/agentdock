package oauthclient

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"
)

const (
	flowTTL             = 10 * time.Minute
	flowStatusRetention = 15 * time.Minute
	maxFlowStatuses     = 128
	exchangeTTL         = 30 * time.Second
	httpTimeout         = 20 * time.Second
)

type challengeState struct {
	endpoint string
	headers  []string
}

type flow struct {
	id               string
	generation       uint64
	epoch            string
	state            string
	server           string
	storageKey       string
	endpoint         string
	callbackID       string
	redirectURL      string
	resource         string
	issuer           string
	issuerInResponse bool
	registrationKey  string
	client           ClientRecord
	tokenURL         string
	requestedScopes  []string
	verifier         string
	authorizationURL string
	expiresAt        time.Time
	callback         chan CallbackResult
	cancel           chan struct{}
	done             chan error
	delivered        bool
}

type flowObservation struct {
	status      string
	expiresAt   time.Time
	completedAt time.Time
	errorCode   string
}

type Manager struct {
	store      *store
	httpClient *http.Client

	mu              sync.Mutex
	registrationMu  sync.Mutex
	callbacks       map[string]CallbackOption
	challenges      map[string]challengeState
	flows           map[string]*flow
	activeByStorage map[string]string
	beginning       map[string]uint64
	flowStatuses    map[string]flowObservation
	// mu serializes generation checks and local grant persistence, never network I/O.
	generations map[string]uint64
}

func New(agentDockHome string) (*Manager, error) {
	store, err := newStore(agentDockHome)
	if err != nil {
		return nil, err
	}
	return &Manager{
		store:           store,
		httpClient:      &http.Client{Timeout: httpTimeout},
		callbacks:       make(map[string]CallbackOption),
		challenges:      make(map[string]challengeState),
		flows:           make(map[string]*flow),
		activeByStorage: make(map[string]string),
		beginning:       make(map[string]uint64),
		flowStatuses:    make(map[string]flowObservation),
		generations:     make(map[string]uint64),
	}, nil
}

func (m *Manager) SetCallback(option CallbackOption) error {
	option.ID = strings.TrimSpace(option.ID)
	option.Label = strings.TrimSpace(option.Label)
	option.RedirectURL = strings.TrimSpace(option.RedirectURL)
	if err := validateCallback(option); err != nil {
		return err
	}
	m.mu.Lock()
	m.callbacks[option.ID] = option
	m.mu.Unlock()
	return nil
}

func (m *Manager) RemoveCallback(id string) {
	m.mu.Lock()
	delete(m.callbacks, strings.TrimSpace(id))
	m.mu.Unlock()
}

func (m *Manager) CallbackOptions() []CallbackOption {
	m.mu.Lock()
	defer m.mu.Unlock()
	return cloneCallbackOptions(m.callbacks)
}

func (m *Manager) RecordChallenge(storageKey, endpoint string, headers []string) {
	storageKey = strings.TrimSpace(storageKey)
	endpoint = strings.TrimSpace(endpoint)
	if storageKey == "" || endpoint == "" {
		return
	}
	m.mu.Lock()
	m.challenges[storageKey] = challengeState{endpoint: endpoint, headers: append([]string(nil), headers...)}
	m.mu.Unlock()
}

func (m *Manager) Status(storageKey, endpoint string) string {
	storageKey = strings.TrimSpace(storageKey)
	endpoint = strings.TrimSpace(endpoint)
	m.mu.Lock()
	if state := m.activeByStorage[storageKey]; state != "" {
		if f := m.flows[state]; f != nil && time.Now().Before(f.expiresAt) && m.store.checkEpoch(storageKey, f.epoch) == nil {
			m.mu.Unlock()
			return StatusAuthorizing
		}
	}
	_, challenged := m.challenges[storageKey]
	m.mu.Unlock()
	grant, err := m.store.loadGrant(storageKey)
	if err == nil && grant.Endpoint == endpoint && strings.TrimSpace(grant.AccessToken) != "" {
		return StatusAuthorized
	}
	if challenged {
		return StatusAuthRequired
	}
	return StatusUnauthorized
}

func (m *Manager) ReserveAuthorization(server, storageKey, endpoint, callbackID string) (*AuthorizationReservation, error) {
	server = strings.TrimSpace(server)
	storageKey = strings.TrimSpace(storageKey)
	endpoint = strings.TrimSpace(endpoint)
	callbackID = strings.TrimSpace(callbackID)
	if server == "" || storageKey == "" || endpoint == "" {
		return nil, newFlowError("MCP_AUTH_INVALID", "OAuth authorization requires server, storage key, and endpoint", nil)
	}

	m.mu.Lock()
	m.cleanupExpiredLocked(time.Now())
	if state := m.activeByStorage[storageKey]; state != "" {
		if existing := m.flows[state]; existing != nil && existing.endpoint == endpoint && m.store.checkEpoch(storageKey, existing.epoch) == nil {
			result := BeginResult{
				FlowID:           existing.id,
				AuthorizationURL: existing.authorizationURL,
				CallbackID:       existing.callbackID,
				ExpiresAt:        formatExpiry(existing.expiresAt),
			}
			done := existing.done
			m.mu.Unlock()
			return &AuthorizationReservation{manager: m, storageKey: storageKey, epoch: existing.epoch, result: result, done: done}, nil
		}
	}
	options := cloneCallbackOptions(m.callbacks)
	challenge := m.challenges[storageKey]
	m.mu.Unlock()

	selected, selectionRequired, err := selectCallback(options, callbackID)
	if err != nil {
		return nil, err
	}
	if selectionRequired {
		return &AuthorizationReservation{manager: m, result: BeginResult{CallbackOptions: options}}, nil
	}

	// discovery / DCR 会发网络请求，不能一直持有 Manager 锁；但同一 StorageKey
	// 在这个窗口也不能启动第二次注册。先做一个短生命周期 reservation，真正
	// Flow 建立后再由 activeByStorage 接管。
	m.mu.Lock()
	if state := m.activeByStorage[storageKey]; state != "" {
		if existing := m.flows[state]; existing != nil && existing.endpoint == endpoint && m.store.checkEpoch(storageKey, existing.epoch) == nil {
			result := BeginResult{FlowID: existing.id, AuthorizationURL: existing.authorizationURL, CallbackID: existing.callbackID, ExpiresAt: formatExpiry(existing.expiresAt)}
			done := existing.done
			m.mu.Unlock()
			return &AuthorizationReservation{manager: m, storageKey: storageKey, epoch: existing.epoch, result: result, done: done}, nil
		}
	}
	m.cleanupFlowStatusesLocked(time.Now().UTC())
	if len(m.flowStatuses) >= maxFlowStatuses {
		m.mu.Unlock()
		return nil, newFlowError("MCP_AUTH_FLOW_LIMIT", "OAuth authorization flow capacity is full", nil)
	}
	if _, busy := m.beginning[storageKey]; busy {
		m.mu.Unlock()
		return nil, newFlowError("MCP_AUTH_IN_PROGRESS", "OAuth authorization is already being prepared", nil)
	}
	epoch, err := m.store.reserve(storageKey)
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}
	m.generations[storageKey]++
	generation := m.generations[storageKey]
	m.beginning[storageKey] = generation
	m.mu.Unlock()
	return &AuthorizationReservation{manager: m, server: server, storageKey: storageKey, endpoint: endpoint, selected: selected, challenge: challenge, generation: generation, epoch: epoch}, nil
}

// AuthorizationReservation binds captured configuration to a durable incarnation.
// Its fields are private so callers cannot substitute an endpoint after reservation.
type AuthorizationReservation struct {
	manager                             *Manager
	server, storageKey, endpoint, epoch string
	selected                            CallbackOption
	challenge                           challengeState
	generation                          uint64
	result                              BeginResult
	done                                <-chan error
}

func (m *Manager) Begin(ctx context.Context, server, storageKey, endpoint, callbackID string) (BeginResult, <-chan error, error) {
	reservation, err := m.ReserveAuthorization(server, storageKey, endpoint, callbackID)
	if err != nil {
		return BeginResult{}, nil, err
	}
	return m.BeginReserved(ctx, reservation)
}

// BeginReserved performs discovery and registration only after the caller has
// released its configuration lock. Invalidation never renews this reservation.
func (m *Manager) BeginReserved(ctx context.Context, r *AuthorizationReservation) (BeginResult, <-chan error, error) {
	if r == nil || r.manager != m {
		return BeginResult{}, nil, newFlowError("MCP_AUTH_CANCELLED", "OAuth authorization flow was cancelled", nil)
	}
	if r.epoch == "" {
		return r.result, r.done, nil
	}
	server, storageKey, endpoint, generation := r.server, r.storageKey, r.endpoint, r.generation
	selected, challenge := r.selected, r.challenge
	if r.result.AuthorizationURL != "" {
		if err := m.store.checkEpoch(storageKey, r.epoch); err != nil {
			return BeginResult{}, nil, newFlowError("MCP_AUTH_CANCELLED", "OAuth authorization flow was cancelled", nil)
		}
		return r.result, r.done, nil
	}
	flowCreated := false
	defer func() {
		if flowCreated {
			return
		}
		m.mu.Lock()
		if m.beginning[storageKey] == generation {
			delete(m.beginning, storageKey)
		}
		m.mu.Unlock()
	}()

	if err := m.store.checkEpoch(storageKey, r.epoch); err != nil {
		return BeginResult{}, nil, newFlowError("MCP_AUTH_CANCELLED", "OAuth authorization flow was cancelled", nil)
	}

	discovered, err := discoverAuthorization(ctx, m.httpClient, endpoint, challenge.headers)
	if err != nil {
		return BeginResult{}, nil, newFlowError("MCP_AUTH_DISCOVERY_FAILED", "discover Remote MCP OAuth metadata", err)
	}
	if existing, grantErr := m.store.loadGrant(storageKey); grantErr == nil {
		if existing.Endpoint == endpoint && existing.Resource == discovered.Resource && existing.Issuer == discovered.Issuer {
			// Step-up authorization 不能因为服务器只 challenge 新增 scope 就丢掉
			// 之前已授予的 scope；与 go-sdk SEP-2350 行为保持一致。
			discovered.RequestedScopes = uniqueStrings(append(append([]string(nil), existing.GrantedScopes...), discovered.RequestedScopes...))
		}
	} else if !errors.Is(grantErr, os.ErrNotExist) {
		return BeginResult{}, nil, newFlowError("MCP_AUTH_PERSIST_FAILED", "read existing OAuth grant", grantErr)
	}
	client, key, err := m.clientFor(ctx, discovered, selected)
	if err != nil {
		return BeginResult{}, nil, err
	}

	state, err := randomState()
	if err != nil {
		return BeginResult{}, nil, newFlowError("MCP_AUTH_FAILED", "generate OAuth state", err)
	}
	flowID, err := randomState()
	if err != nil {
		return BeginResult{}, nil, newFlowError("MCP_AUTH_FAILED", "generate OAuth flow id", err)
	}
	verifier := oauth2.GenerateVerifier()
	config := oauthConfig(client, discovered.Metadata.TokenEndpoint, selected.RedirectURL, discovered.RequestedScopes)
	config.Endpoint.AuthURL = discovered.Metadata.AuthorizationEndpoint
	authOptions := []oauth2.AuthCodeOption{oauth2.S256ChallengeOption(verifier)}
	if discovered.Resource != "" {
		authOptions = append(authOptions, oauth2.SetAuthURLParam("resource", discovered.Resource))
	}
	authorizationURL := config.AuthCodeURL(state, authOptions...)
	now := time.Now().UTC()
	f := &flow{
		id: flowID, state: state, server: server, storageKey: storageKey, endpoint: endpoint, generation: generation, epoch: r.epoch,
		callbackID: selected.ID, redirectURL: selected.RedirectURL,
		resource: discovered.Resource, issuer: discovered.Issuer,
		issuerInResponse: discovered.Metadata.AuthorizationResponseIssParameterSupported,
		registrationKey:  key, client: client, tokenURL: discovered.Metadata.TokenEndpoint,
		requestedScopes: append([]string(nil), discovered.RequestedScopes...), verifier: verifier,
		authorizationURL: authorizationURL, expiresAt: now.Add(flowTTL),
		callback: make(chan CallbackResult, 1), cancel: make(chan struct{}), done: make(chan error, 1),
	}
	m.mu.Lock()
	if m.generations[storageKey] != generation || m.beginning[storageKey] != generation || m.store.checkEpoch(storageKey, r.epoch) != nil {
		m.mu.Unlock()
		return BeginResult{}, nil, newFlowError("MCP_AUTH_CANCELLED", "OAuth authorization flow was cancelled", nil)
	}
	m.flows[state] = f
	m.activeByStorage[storageKey] = state
	m.flowStatuses[flowID] = flowObservation{status: "authorizing", expiresAt: f.expiresAt}
	m.cleanupFlowStatusesLocked(now)
	delete(m.beginning, storageKey)
	m.mu.Unlock()
	flowCreated = true
	go m.completeFlow(f)
	return BeginResult{FlowID: flowID, AuthorizationURL: authorizationURL, CallbackID: selected.ID, ExpiresAt: formatExpiry(f.expiresAt)}, f.done, nil
}

func (m *Manager) FlowStatus(flowID string) (FlowStatus, bool) {
	flowID = strings.TrimSpace(flowID)
	if flowID == "" {
		return FlowStatus{}, false
	}
	now := time.Now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupFlowStatusesLocked(now)
	observation, ok := m.flowStatuses[flowID]
	if !ok {
		return FlowStatus{}, false
	}
	return FlowStatus{FlowID: flowID, Status: observation.status, ExpiresAt: formatExpiry(observation.expiresAt), ErrorCode: observation.errorCode}, true
}

func (m *Manager) DeliverCallback(result CallbackResult) error {
	result.State = strings.TrimSpace(result.State)
	if result.State == "" {
		return newFlowError("MCP_AUTH_STATE_MISMATCH", "OAuth callback omitted state", nil)
	}
	m.mu.Lock()
	f := m.flows[result.State]
	if f == nil || time.Now().After(f.expiresAt) {
		m.mu.Unlock()
		return newFlowError("MCP_AUTH_STATE_MISMATCH", "OAuth callback state is unknown or expired", nil)
	}
	if f.delivered {
		m.mu.Unlock()
		return newFlowError("MCP_AUTH_STATE_MISMATCH", "OAuth callback state was already consumed", nil)
	}
	f.delivered = true
	m.mu.Unlock()
	f.callback <- result
	return nil
}

func (m *Manager) Clear(storageKey string) error {
	storageKey = strings.TrimSpace(storageKey)
	m.mu.Lock()
	defer m.mu.Unlock()
	err := m.store.removeGrant(storageKey)
	m.generations[storageKey]++
	delete(m.beginning, storageKey)
	delete(m.challenges, storageKey)
	if state := m.activeByStorage[storageKey]; state != "" {
		if f := m.flows[state]; f != nil {
			delete(m.flows, state)
			delete(m.activeByStorage, storageKey)
			close(f.cancel)
		}
	}
	return err
}

func (m *Manager) RemoveGrant(storageKey string) error {
	return m.Clear(storageKey)
}

func (m *Manager) Handler(server, storageKey, endpoint string) *Handler {
	return &Handler{manager: m, server: strings.TrimSpace(server), storageKey: strings.TrimSpace(storageKey), endpoint: strings.TrimSpace(endpoint)}
}

func (m *Manager) completeFlow(f *flow) {
	var err error
	select {
	case result := <-f.callback:
		err = m.exchange(f, result)
	case <-time.After(time.Until(f.expiresAt)):
		err = newFlowError("MCP_AUTH_EXPIRED", "OAuth authorization flow expired", nil)
	case <-f.cancel:
		err = newFlowError("MCP_AUTH_CANCELLED", "OAuth authorization flow was cancelled", nil)
	}
	m.mu.Lock()
	now := time.Now().UTC()
	if m.flows[f.state] == f {
		delete(m.flows, f.state)
	}
	if m.activeByStorage[f.storageKey] == f.state {
		delete(m.activeByStorage, f.storageKey)
	}
	if err == nil && m.generations[f.storageKey] == f.generation {
		delete(m.challenges, f.storageKey)
	}
	status, errorCode := terminalFlowStatus(err)
	if f.id != "" {
		observation := m.flowStatuses[f.id]
		observation.status, observation.errorCode, observation.completedAt = status, errorCode, now
		if observation.expiresAt.IsZero() {
			observation.expiresAt = f.expiresAt
		}
		m.flowStatuses[f.id] = observation
		m.cleanupFlowStatusesLocked(now)
	}
	m.mu.Unlock()
	f.done <- err
	close(f.done)
}

func terminalFlowStatus(err error) (string, string) {
	if err == nil {
		return "authorized", ""
	}
	var flowErr *FlowError
	if !errors.As(err, &flowErr) {
		return "failed", "MCP_AUTH_FAILED"
	}
	switch flowErr.Code {
	case "MCP_AUTH_DENIED":
		return "denied", flowErr.Code
	case "MCP_AUTH_EXPIRED":
		return "expired", flowErr.Code
	case "MCP_AUTH_CANCELLED":
		return "cancelled", flowErr.Code
	default:
		return "failed", flowErr.Code
	}
}

func (m *Manager) cleanupFlowStatusesLocked(now time.Time) {
	cutoff := now.Add(-flowStatusRetention)
	for id, observation := range m.flowStatuses {
		if !observation.completedAt.IsZero() && observation.completedAt.Before(cutoff) {
			delete(m.flowStatuses, id)
		}
	}
	if len(m.flowStatuses) <= maxFlowStatuses {
		return
	}
	type completed struct {
		id string
		at time.Time
	}
	items := make([]completed, 0, len(m.flowStatuses))
	for id, observation := range m.flowStatuses {
		if !observation.completedAt.IsZero() {
			items = append(items, completed{id: id, at: observation.completedAt})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].at.Before(items[j].at) })
	for _, item := range items {
		if len(m.flowStatuses) <= maxFlowStatuses {
			break
		}
		delete(m.flowStatuses, item.id)
	}
}

func (m *Manager) exchange(f *flow, result CallbackResult) error {
	if strings.TrimSpace(result.Error) != "" {
		if strings.EqualFold(strings.TrimSpace(result.Error), "access_denied") {
			return newFlowError("MCP_AUTH_DENIED", "OAuth authorization was denied", nil)
		}
		return newFlowError("MCP_AUTH_FAILED", "OAuth authorization failed", nil)
	}
	if strings.TrimSpace(result.Code) == "" {
		return newFlowError("MCP_AUTH_FAILED", "OAuth callback omitted authorization code", nil)
	}
	if err := validateIssuer(result.Issuer, f.issuer, f.issuerInResponse); err != nil {
		return newFlowError("MCP_AUTH_ISSUER_MISMATCH", "OAuth callback issuer did not match authorization server", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), exchangeTTL)
	defer cancel()
	ctx = context.WithValue(ctx, oauth2.HTTPClient, m.httpClient)
	config := oauthConfig(f.client, f.tokenURL, f.redirectURL, f.requestedScopes)
	exchangeOptions := []oauth2.AuthCodeOption{oauth2.VerifierOption(f.verifier)}
	if f.resource != "" {
		exchangeOptions = append(exchangeOptions, oauth2.SetAuthURLParam("resource", f.resource))
	}
	token, err := config.Exchange(ctx, result.Code, exchangeOptions...)
	if err != nil {
		return newFlowError("MCP_AUTH_TOKEN_EXCHANGE_FAILED", "exchange OAuth authorization code", err)
	}
	scopes := append([]string(nil), f.requestedScopes...)
	if raw, ok := token.Extra("scope").(string); ok && strings.TrimSpace(raw) != "" {
		scopes = strings.Fields(raw)
	}
	grant := Grant{
		Endpoint: f.endpoint, Resource: f.resource, Issuer: f.issuer, RedirectURL: f.redirectURL,
		RegistrationKey: f.registrationKey, TokenURL: f.tokenURL,
		AccessToken: token.AccessToken, RefreshToken: token.RefreshToken, TokenType: token.TokenType,
		Expiry: token.Expiry, GrantedScopes: uniqueStrings(scopes),
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.generations[f.storageKey] != f.generation || m.flows[f.state] != f || m.activeByStorage[f.storageKey] != f.state {
		return newFlowError("MCP_AUTH_CANCELLED", "OAuth authorization flow was cancelled", nil)
	}
	if err := m.store.saveGrantChecked(f.storageKey, f.epoch, grant); err != nil {
		var required *AuthRequiredError
		if errors.As(err, &required) {
			return newFlowError("MCP_AUTH_CANCELLED", "OAuth authorization flow was cancelled", nil)
		}
		return newFlowError("MCP_AUTH_PERSIST_FAILED", "persist OAuth token", err)
	}
	m.store.touchClient(f.registrationKey)
	return nil
}

func (m *Manager) clientFor(ctx context.Context, discovered discoveredAuth, callback CallbackOption) (ClientRecord, string, error) {
	// DCR is extremely rare compared with normal MCP calls. Serialize registration so two
	// different MCPs sharing the same issuer+redirect cannot both register and race to
	// overwrite clients.json with different client_ids. Re-check persisted state while held.
	m.registrationMu.Lock()
	defer m.registrationMu.Unlock()

	key := registrationKey(discovered.Issuer, callback.RedirectURL)
	if client, err := m.store.loadClient(key); err == nil {
		if client.Issuer == discovered.Issuer && client.RedirectURL == callback.RedirectURL &&
			(client.ClientSecretExpiresAt.IsZero() || time.Now().Before(client.ClientSecretExpiresAt)) {
			return client, key, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return ClientRecord{}, "", newFlowError("MCP_AUTH_REGISTRATION_FAILED", "read persisted OAuth client registration", err)
	}
	if strings.TrimSpace(discovered.Metadata.RegistrationEndpoint) == "" {
		return ClientRecord{}, "", newFlowError("MCP_AUTH_REGISTRATION_REQUIRED", "authorization server does not support Dynamic Client Registration", nil)
	}
	method := preferredRegistrationAuthMethod(discovered.Metadata.TokenEndpointAuthMethodsSupported)
	if method == "" {
		return ClientRecord{}, "", newFlowError("MCP_AUTH_REGISTRATION_FAILED", "authorization server only advertises unsupported token endpoint authentication methods", nil)
	}
	grantTypes := []string{"authorization_code"}
	if slices.Contains(discovered.Metadata.GrantTypesSupported, "refresh_token") {
		grantTypes = append(grantTypes, "refresh_token")
	}
	registration, err := oauthex.RegisterClient(ctx, discovered.Metadata.RegistrationEndpoint, &oauthex.ClientRegistrationMetadata{
		RedirectURIs:            []string{callback.RedirectURL},
		TokenEndpointAuthMethod: method,
		GrantTypes:              grantTypes,
		ResponseTypes:           []string{"code"},
		ClientName:              "AgentDock",
		ApplicationType:         applicationType(callback.RedirectURL),
	}, m.httpClient)
	if err != nil {
		return ClientRecord{}, "", newFlowError("MCP_AUTH_REGISTRATION_FAILED", "register OAuth client", err)
	}
	client := ClientRecord{
		Issuer: discovered.Issuer, RedirectURL: callback.RedirectURL,
		ClientID: registration.ClientID, ClientSecret: registration.ClientSecret,
		TokenEndpointAuthMethod: registration.TokenEndpointAuthMethod,
		ClientIDIssuedAt:        registration.ClientIDIssuedAt, ClientSecretExpiresAt: registration.ClientSecretExpiresAt,
	}
	if client.TokenEndpointAuthMethod == "" {
		client.TokenEndpointAuthMethod = method
	}
	if err := m.store.saveClient(key, client); err != nil {
		return ClientRecord{}, "", newFlowError("MCP_AUTH_REGISTRATION_FAILED", "persist OAuth client registration", err)
	}
	return client, key, nil
}

func (m *Manager) tokenSource(storageKey, endpoint string) (oauth2.TokenSource, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	grant, epoch, err := m.store.loadGrantWithEpoch(storageKey)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if grant.Endpoint != endpoint || strings.TrimSpace(grant.AccessToken) == "" {
		return nil, nil
	}
	if !grant.Expiry.IsZero() && time.Now().After(grant.Expiry) && strings.TrimSpace(grant.RefreshToken) == "" {
		_ = m.store.clearChecked(storageKey, &epoch)
		return nil, nil
	}
	client, err := m.store.loadClient(grant.RegistrationKey)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	config := oauthConfig(client, grant.TokenURL, grant.RedirectURL, grant.GrantedScopes)
	refreshClient := *m.httpClient
	baseTransport := refreshClient.Transport
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}
	refreshClient.Transport = resourceRefreshRoundTripper{base: baseTransport, tokenURL: grant.TokenURL, resource: grant.Resource}
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, &refreshClient)
	base := config.TokenSource(ctx, tokenFromGrant(grant))
	return &persistingTokenSource{source: base, manager: m, epoch: epoch, generation: m.generations[storageKey], storageKey: storageKey, grant: grant}, nil
}

func (m *Manager) cleanupExpiredLocked(now time.Time) {
	for state, f := range m.flows {
		if now.Before(f.expiresAt) {
			continue
		}
		delete(m.flows, state)
		if m.activeByStorage[f.storageKey] == state {
			delete(m.activeByStorage, f.storageKey)
		}
	}
}

func validateCallback(option CallbackOption) error {
	switch option.ID {
	case CallbackLocal, CallbackAgentDock, CallbackNexus:
	default:
		return fmt.Errorf("unsupported OAuth callback id %q", option.ID)
	}
	if option.Label == "" || option.RedirectURL == "" {
		return errors.New("OAuth callback label and redirect URL are required")
	}
	u, err := url.Parse(option.RedirectURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("invalid OAuth redirect URL %q", option.RedirectURL)
	}
	if option.ID == CallbackLocal {
		if u.Scheme != "http" || !isLoopbackHost(u.Hostname()) {
			return fmt.Errorf("local OAuth callback must use loopback HTTP: %q", option.RedirectURL)
		}
		return nil
	}
	if u.Scheme != "https" {
		return fmt.Errorf("public OAuth callback must use HTTPS: %q", option.RedirectURL)
	}
	return nil
}

func selectCallback(options []CallbackOption, id string) (CallbackOption, bool, error) {
	if len(options) == 0 {
		return CallbackOption{}, false, newFlowError("MCP_AUTH_CALLBACK_UNAVAILABLE", "no OAuth callback is currently available", nil)
	}
	if id == "" {
		if len(options) == 1 {
			return options[0], false, nil
		}
		return CallbackOption{}, true, nil
	}
	for _, option := range options {
		if option.ID == id {
			return option, false, nil
		}
	}
	return CallbackOption{}, false, newFlowError("MCP_AUTH_CALLBACK_INVALID", "requested OAuth callback is not available", nil)
}

func cloneCallbackOptions(input map[string]CallbackOption) []CallbackOption {
	result := make([]CallbackOption, 0, len(input))
	for _, option := range input {
		result = append(result, option)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func randomState() (string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func preferredRegistrationAuthMethod(supported []string) string {
	for _, method := range []string{"none", "client_secret_post", "client_secret_basic"} {
		if slices.Contains(supported, method) {
			return method
		}
	}
	if len(supported) == 0 {
		// Remote MCP DCR clients are public PKCE clients unless the server explicitly
		// advertises a confidential-client method.
		return "none"
	}
	return ""
}

func oauthConfig(client ClientRecord, tokenURL, redirectURL string, scopes []string) *oauth2.Config {
	return &oauth2.Config{
		ClientID: client.ClientID, ClientSecret: client.ClientSecret,
		Endpoint:    oauth2.Endpoint{AuthURL: "", TokenURL: tokenURL, AuthStyle: authStyle(client)},
		RedirectURL: redirectURL, Scopes: append([]string(nil), scopes...),
	}
}

func authStyle(client ClientRecord) oauth2.AuthStyle {
	switch client.TokenEndpointAuthMethod {
	case "client_secret_post", "none":
		return oauth2.AuthStyleInParams
	case "client_secret_basic":
		return oauth2.AuthStyleInHeader
	default:
		if client.ClientSecret == "" {
			return oauth2.AuthStyleInParams
		}
		return oauth2.AuthStyleAutoDetect
	}
}

func applicationType(redirectURL string) string {
	u, err := url.Parse(redirectURL)
	if err == nil && u.Scheme == "http" && isLoopbackHost(u.Hostname()) {
		return "native"
	}
	return "web"
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(strings.TrimSpace(host), "localhost") {
		return true
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	return ip != nil && ip.IsLoopback()
}

func validateIssuer(actual, expected string, required bool) error {
	actual = strings.TrimSpace(actual)
	expected = strings.TrimRight(strings.TrimSpace(expected), "/")
	if actual == "" {
		if required {
			return errors.New("authorization response omitted required iss parameter")
		}
		return nil
	}
	if strings.TrimRight(actual, "/") != expected {
		return fmt.Errorf("authorization response issuer %q does not match %q", actual, expected)
	}
	return nil
}

type resourceRefreshRoundTripper struct {
	base     http.RoundTripper
	tokenURL string
	resource string
}

func (t resourceRefreshRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	if request == nil || request.Body == nil || request.Method != http.MethodPost || strings.TrimSpace(t.resource) == "" || request.URL == nil || request.URL.String() != t.tokenURL {
		return base.RoundTrip(request)
	}
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	values, err := url.ParseQuery(string(body))
	if err != nil || values.Get("grant_type") != "refresh_token" {
		clone := request.Clone(request.Context())
		clone.Body = io.NopCloser(bytes.NewReader(body))
		clone.ContentLength = int64(len(body))
		return base.RoundTrip(clone)
	}
	if values.Get("resource") == "" {
		values.Set("resource", t.resource)
	}
	encoded := []byte(values.Encode())
	clone := request.Clone(request.Context())
	clone.Body = io.NopCloser(bytes.NewReader(encoded))
	clone.ContentLength = int64(len(encoded))
	clone.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(encoded)), nil }
	return base.RoundTrip(clone)
}

type persistingTokenSource struct {
	mu         sync.Mutex
	source     oauth2.TokenSource
	manager    *Manager
	generation uint64
	epoch      string
	storageKey string
	grant      Grant
}

func (s *persistingTokenSource) Token() (*oauth2.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.manager.mu.Lock()
	valid := s.manager.generations[s.storageKey] == s.generation
	s.manager.mu.Unlock()
	if !valid || s.manager.store.checkEpoch(s.storageKey, s.epoch) != nil {
		return nil, &AuthRequiredError{}
	}
	token, err := s.source.Token()
	s.manager.mu.Lock()
	defer s.manager.mu.Unlock()
	if s.manager.generations[s.storageKey] != s.generation || s.manager.store.checkEpoch(s.storageKey, s.epoch) != nil {
		return nil, &AuthRequiredError{}
	}
	if err != nil {
		var retrieveErr *oauth2.RetrieveError
		if errors.As(err, &retrieveErr) && retrieveErr.ErrorCode == "invalid_grant" {
			_ = s.manager.store.clearChecked(s.storageKey, &s.epoch)
			return nil, &AuthRequiredError{}
		}
		return nil, err
	}
	if grantTokenChanged(s.grant, token) {
		s.grant.AccessToken = token.AccessToken
		s.grant.RefreshToken = token.RefreshToken
		s.grant.TokenType = token.TokenType
		s.grant.Expiry = token.Expiry
		if err := s.manager.store.saveGrantChecked(s.storageKey, s.epoch, s.grant); err != nil {
			return nil, err
		}
	}
	return token, nil
}
