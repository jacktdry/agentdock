package browserpolicy

import "testing"

func TestOriginScopeCanonicalizationAndMatching(t *testing.T) {
	scope, err := NewOriginScope([]string{"https://z.example.com", "https://cms.example.com:8443"})
	if err != nil {
		t.Fatal(err)
	}
	reordered, err := NewOriginScope([]string{"https://cms.example.com:8443", "https://z.example.com"})
	if err != nil || reordered != scope {
		t.Fatalf("scope order changed authority: %q %v", reordered, err)
	}
	for _, raw := range []string{
		"https://z.example.com/path?q=1#fragment",
		"https://z.example.com:443/path",
		"https://cms.example.com:8443/admin",
	} {
		if !scope.Allows(raw) {
			t.Fatalf("allowed URL rejected: %s", raw)
		}
	}
	for _, raw := range []string{
		"", "about:blank", "http://z.example.com/", "https://evil.example.com/",
		"https://z.example.com.evil/", "https://user@z.example.com/", " https://z.example.com/",
	} {
		if scope.Allows(raw) {
			t.Fatalf("out-of-scope URL accepted: %q", raw)
		}
	}
}

func TestOriginScopeRejectsNonCanonicalConfiguration(t *testing.T) {
	for _, raw := range []string{
		"http://cms.example.com", "https://CMS.example.com", "https://cms.example.com/",
		"https://cms.example.com:443", "https://user@cms.example.com",
		"https://cms.example.com/path", "https://cms.example.com?q=1",
	} {
		t.Run(raw, func(t *testing.T) {
			if _, err := NewOriginScope([]string{raw}); err == nil {
				t.Fatal("non-canonical origin accepted")
			}
		})
	}
	if _, err := NewOriginScope([]string{"https://cms.example.com", "https://cms.example.com"}); err == nil {
		t.Fatal("duplicate origin accepted")
	}
}

func TestCompanyPolicyRequiresTrustedOriginScope(t *testing.T) {
	p := BrowserRoutePolicy{
		Class: WorkspaceCompany, Route: RouteRequiredExternal,
		RequiredConnectorID: "edge", RequiredProfileID: "company-edge",
	}
	if err := ValidateRoutePolicy(p); err == nil {
		t.Fatal("company policy without origin scope accepted")
	}
	p.AllowedOrigins = []string{"https://cms.example.com"}
	if err := ValidateRoutePolicy(p); err != nil {
		t.Fatal(err)
	}
	equivalent := p
	equivalent.AllowedOrigins = []string{"https://cms.example.com"}
	if !routePoliciesEqual(p, equivalent) {
		t.Fatal("equivalent origin scope changed policy equality")
	}
}
