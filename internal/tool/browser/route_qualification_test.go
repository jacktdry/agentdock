package browser

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestPlannerQualificationRejectsUntrustedEvidence(t *testing.T) {
	cases := map[string]func(*ConnectorRuntimeStatus){
		"perfect raw status":  func(s *ConnectorRuntimeStatus) { *s = unqualifiedRuntimeStatus() },
		"missing proof":       func(s *ConnectorRuntimeStatus) { s.qualification = nil },
		"unsupported proof":   func(s *ConnectorRuntimeStatus) { s.qualification.format = 2 },
		"missing source":      func(s *ConnectorRuntimeStatus) { s.source = nil },
		"wrong source":        func(s *ConnectorRuntimeStatus) { s.source = &connectorEvidenceIdentity{} },
		"missing incarnation": func(s *ConnectorRuntimeStatus) { s.incarnation = nil },
		"wrong incarnation":   func(s *ConnectorRuntimeStatus) { s.incarnation = &connectorEvidenceIdentity{} },
		"source is not incarnation": func(s *ConnectorRuntimeStatus) {
			s.incarnation = s.source
			s.qualification.observation.incarnation = s.source
		},
		"proof missing source":      func(s *ConnectorRuntimeStatus) { s.qualification.observation.source = nil },
		"proof missing incarnation": func(s *ConnectorRuntimeStatus) { s.qualification.observation.incarnation = nil },
		"cross connector proof":     func(s *ConnectorRuntimeStatus) { s.qualification.observation.ConnectorID = "other" },
		"missing proof connector":   func(s *ConnectorRuntimeStatus) { s.qualification.observation.ConnectorID = "" },
		"cross profile proof":       func(s *ConnectorRuntimeStatus) { s.qualification.observation.ProfileID = "other" },
		"missing proof profile":     func(s *ConnectorRuntimeStatus) { s.qualification.observation.ProfileID = "" },
		"cross endpoint proof":      func(s *ConnectorRuntimeStatus) { s.qualification.observation.Endpoint += "/other" },
		"missing proof endpoint":    func(s *ConnectorRuntimeStatus) { s.qualification.observation.Endpoint = "" },
		"renew raw time cannot renew proof": func(s *ConnectorRuntimeStatus) {
			s.qualification.observation.ObservedAt = time.Now().UTC().Add(-10 * time.Second)
			s.qualification.observation.ExpiresAt = time.Now().UTC().Add(-6 * time.Second)
		},
		"future proof": func(s *ConnectorRuntimeStatus) {
			s.qualification.observation.ObservedAt = time.Now().UTC().Add(time.Second)
		},
		"non UTC proof": func(s *ConnectorRuntimeStatus) {
			s.qualification.observation.ObservedAt = s.ObservedAt.In(time.FixedZone("offset", 3600))
		},
		"empty proof":            func(s *ConnectorRuntimeStatus) { s.qualification = &connectorQualification{} },
		"upgrade authentication": func(s *ConnectorRuntimeStatus) { s.qualification.observation.Authenticated = false },
		"upgrade health":         func(s *ConnectorRuntimeStatus) { s.qualification.observation.Healthy = false },
		"upgrade verification":   func(s *ConnectorRuntimeStatus) { s.qualification.observation.Verified = false },
		"upgrade capabilities":   func(s *ConnectorRuntimeStatus) { s.qualification.observation.Capabilities = nil },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s := validRuntimeStatus()
			mutate(&s)
			planner, scope := plannerFixture(t, testConnectorStatus(func(context.Context, string) (ConnectorRuntimeStatus, error) { return s, nil }))
			d, err := planner.Resolve(context.Background(), scope, nil)
			assertRouteCode(t, err, ErrRequiredRouteUnavailable)
			if d != (RouteDecision{}) {
				t.Fatal("untrusted evidence authorized a route or fallback")
			}
		})
	}
}

func TestPlannerQualificationNonceCannotReplayAcrossPlanners(t *testing.T) {
	s := validRuntimeStatus()
	provider := testConnectorStatus(func(context.Context, string) (ConnectorRuntimeStatus, error) { return s, nil })
	first, scope := plannerFixture(t, provider)
	if _, err := first.Resolve(context.Background(), scope, nil); err != nil {
		t.Fatal(err)
	}
	second, otherScope := plannerFixture(t, provider)
	for _, target := range []struct {
		planner *RoutePlanner
		scope   RequestScope
	}{{first, scope}, {second, otherScope}} {
		d, err := target.planner.Resolve(context.Background(), target.scope, nil)
		assertRouteCode(t, err, ErrRequiredRouteUnavailable)
		if d != (RouteDecision{}) {
			t.Fatal("copied nonce replayed")
		}
	}
}

func TestPlannerQualificationCannotSurviveSerialization(t *testing.T) {
	s := validRuntimeStatus()
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var decoded ConnectorRuntimeStatus
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.source != nil || decoded.incarnation != nil || decoded.qualification != nil {
		t.Fatal("authority serialized")
	}
	planner, scope := plannerFixture(t, testConnectorStatus(func(context.Context, string) (ConnectorRuntimeStatus, error) { return decoded, nil }))
	d, err := planner.Resolve(context.Background(), scope, nil)
	assertRouteCode(t, err, ErrRequiredRouteUnavailable)
	if d != (RouteDecision{}) {
		t.Fatal("serialized assertion authorized route")
	}
}
