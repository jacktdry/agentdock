package browserpolicy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspacePolicies(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "workspace")
	nested := filepath.Join(root, "nested")
	sibling := filepath.Join(base, "workspace-other")
	for _, p := range []string{nested, sibling} {
		if err := os.MkdirAll(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	policy := BrowserRoutePolicy{Class: WorkspaceCompany, Route: RouteRequiredExternal, RequiredConnectorID: "connector", RequiredProfileID: "profile"}
	mappings := []WorkspaceRootPolicy{{Root: root, Policy: policy}}
	cases := []struct {
		name, path string
		company    bool
	}{
		{"root", root, true}, {"nested", nested, true}, {"cleaned", nested + string(filepath.Separator) + "..", true}, {"sibling prefix", sibling, false}, {"parent", base, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			canonical, p, err := ClassifyWorkspace(tc.path, mappings)
			if err != nil || !filepath.IsAbs(canonical) || (p.Class == WorkspaceCompany) != tc.company {
				t.Fatalf("%q %+v %v", canonical, p, err)
			}
		})
	}
	for _, path := range []string{"relative", filepath.Join(base, "missing")} {
		if _, _, err := ClassifyWorkspace(path, mappings); err == nil {
			t.Fatalf("accepted %q", path)
		}
	}
	if _, err := NormalizeWorkspacePolicies(append(mappings, WorkspaceRootPolicy{Root: nested, Policy: BrowserRoutePolicy{Class: WorkspaceDefault, Route: RouteManaged}})); err == nil {
		t.Fatal("nested policy weakened parent")
	}
	file := filepath.Join(base, "file")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := CanonicalWorkspaceRoot(file); err == nil {
		t.Fatal("accepted file")
	}
}

func TestWorkspaceSymlinkAlias(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "real")
	alias := filepath.Join(base, "alias")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	policy := BrowserRoutePolicy{Class: WorkspaceCompany, Route: RouteRequiredExternal, RequiredConnectorID: "connector", RequiredProfileID: "profile"}
	canonical, p, err := ClassifyWorkspace(root, []WorkspaceRootPolicy{{Root: alias, Policy: policy}})
	want, canonicalErr := CanonicalWorkspaceRoot(alias)
	if err != nil || canonicalErr != nil || canonical != want || p != policy {
		t.Fatalf("%q %+v %v", canonical, p, err)
	}
	if _, err := NormalizeWorkspacePolicies([]WorkspaceRootPolicy{{Root: root, Policy: policy}, {Root: alias, Policy: policy}}); err == nil {
		t.Fatal("duplicate alias accepted")
	}
	outside := filepath.Join(base, "outside")
	if err := os.Mkdir(outside, 0700); err != nil {
		t.Fatal(err)
	}
	escape := filepath.Join(root, "escape")
	if err := os.Symlink(outside, escape); err != nil {
		t.Fatal(err)
	}
	_, p, err = ClassifyWorkspace(escape, []WorkspaceRootPolicy{{Root: root, Policy: policy}})
	if err != nil || p.Class != WorkspaceDefault {
		t.Fatalf("symlink escape misclassified: %+v %v", p, err)
	}
}
