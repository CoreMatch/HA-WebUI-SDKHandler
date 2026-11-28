package inject

import (
	"encoding/json"
	"testing"
)

func TestMenuEntriesUnmarshal(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "single object",
			input:    `{"label": "Home", "path": "/"}`,
			expected: 1,
		},
		{
			name:     "array of objects",
			input:    `[{"label": "Home", "path": "/"}, {"label": "About", "path": "/about"}]`,
			expected: 2,
		},
		{
			name:     "null",
			input:    `null`,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m MenuEntries
			if err := json.Unmarshal([]byte(tt.input), &m); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}
			if len(m) != tt.expected {
				t.Errorf("expected %d entries, got %d", tt.expected, len(m))
			}
		})
	}
}

func TestValidateManifestMultiPage(t *testing.T) {
	m := &Manifest{
		SchemaVersion: 1,
		Name:          "test-svc",
		Version:       "1.0.0",
		Routes: []RouteEntry{
			{Path: "/p1", Module: "src/p1"},
			{Path: "/p2", Module: "src/p2"},
		},
		Menu: []MenuEntry{
			{Label: "Page 1", Path: "/p1"},
			{Label: "Page 2", Path: "/p2"},
		},
		Dashboard: []MenuEntry{
			{Label: "Dash 1", Path: "/p1"},
		},
	}

	errs := ValidateManifest(m)
	if len(errs) > 0 {
		t.Errorf("expected no errors, got %v", errs)
	}

	// Test invalid path in menu
	m.Menu[0].Path = "/invalid"
	errs = ValidateManifest(m)
	if len(errs) == 0 {
		t.Errorf("expected errors for invalid menu path, got none")
	}
}

func TestValidateManifestCriticalOperations(t *testing.T) {
	m := &Manifest{
		SchemaVersion: 1,
		Name:          "test-svc",
		Version:       "1.0.0",
		Routes: []RouteEntry{
			{Path: "/security", Module: "src/security"},
		},
		CriticalOperations: []MenuEntry{
			{Label: "Review security settings", Path: "/security"},
		},
	}

	if errs := ValidateManifest(m); len(errs) > 0 {
		t.Fatalf("expected valid critical operation, got %v", errs)
	}

	m.CriticalOperations[0].Label = " "
	m.CriticalOperations[0].Path = "/missing"
	errs := ValidateManifest(m)
	if len(errs) != 2 {
		t.Fatalf("expected label and route validation errors, got %v", errs)
	}
}

func TestValidateManifestProfileCards(t *testing.T) {
	priority := 5
	m := &Manifest{
		SchemaVersion: 1,
		Name:          "test-svc",
		Version:       "1.0.0",
		ProfileCards: []ProfileCardEntry{
			{Title: "Profile Extensions", Module: "src/profile/CardA", Priority: &priority},
		},
	}

	errs := ValidateManifest(m)
	if len(errs) > 0 {
		t.Errorf("expected no errors, got %v", errs)
	}

	m.ProfileCards[0].Module = "../escape"
	errs = ValidateManifest(m)
	if len(errs) == 0 {
		t.Errorf("expected errors for invalid profile card module, got none")
	}
}

func TestValidateManifestNeedsRoutesOrProfileCards(t *testing.T) {
	m := &Manifest{
		SchemaVersion: 1,
		Name:          "test-svc",
		Version:       "1.0.0",
	}

	errs := ValidateManifest(m)
	if len(errs) == 0 {
		t.Fatalf("expected errors when routes, profile_cards and contributions are all empty")
	}

	found := false
	for _, err := range errs {
		if err == "routes、profile_cards 与 contributions 不能同时为空" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected empty contribution error, got %v", errs)
	}
}

func TestValidateManifestContributions(t *testing.T) {
	priority := 10
	m := &Manifest{
		SchemaVersion: 1,
		Name:          "register-providers",
		Version:       "1.0.0",
		Contributions: []ContributionEntry{
			{
				ID:       "github",
				Surface:  ContributionSurfaceRegisterProviders,
				Module:   "src/register/GitHubButton",
				Label:    "GitHub",
				Priority: &priority,
			},
		},
	}

	if errs := ValidateManifest(m); len(errs) > 0 {
		t.Fatalf("expected contribution-only manifest to be valid, got %v", errs)
	}

	m.Contributions[0].Surface = "unknown.surface"
	m.Contributions[0].Module = "../escape"
	m.Contributions[0].Label = " "
	errs := ValidateManifest(m)
	if len(errs) != 3 {
		t.Fatalf("expected surface, module and label errors, got %v", errs)
	}
}

func TestValidateManifestRejectsDuplicateContributionIDs(t *testing.T) {
	m := &Manifest{
		SchemaVersion: 1,
		Name:          "register-providers",
		Version:       "1.0.0",
		Contributions: []ContributionEntry{
			{ID: "github", Surface: ContributionSurfaceRegisterProviders, Module: "src/GitHub", Label: "GitHub"},
			{ID: "github", Surface: ContributionSurfaceRegisterProviders, Module: "src/GitHubAlt", Label: "GitHub Alt"},
		},
	}

	errs := ValidateManifest(m)
	if len(errs) != 1 {
		t.Fatalf("expected one duplicate ID error, got %v", errs)
	}
}
