package prototypes

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestRegistryNames(t *testing.T) {
	r := NewRegistry()
	got := r.Names()
	want := []string{"component-spec", "interactive-html", "screen-flow", "user-journey"}
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Names() = %v, want %v", got, want)
	}
}

func TestRegistryPick(t *testing.T) {
	r := NewRegistry()

	if got := r.Pick("screen-flow"); got.Name() != "screen-flow" {
		t.Errorf("Pick screen-flow returned %q", got.Name())
	}
	if got := r.Pick("component-spec"); got.Name() != "component-spec" {
		t.Errorf("Pick component-spec returned %q", got.Name())
	}
	if got := r.Pick("user-journey"); got.Name() != "user-journey" {
		t.Errorf("Pick user-journey returned %q", got.Name())
	}

	// Unknown falls back to screen-flow.
	if got := r.Pick("bogus"); got.Name() != "screen-flow" {
		t.Errorf("unknown should fall back to screen-flow, got %q", got.Name())
	}
}

func TestScreenFlowRender(t *testing.T) {
	sf := &ScreenFlow{}
	filled := map[string]interface{}{
		"summary": "Login flow",
		"design_tokens": map[string]interface{}{
			"primary_color": "#0070F3",
		},
		"screens": []interface{}{
			map[string]interface{}{
				"name":       "Sign In",
				"purpose":    "Let users authenticate",
				"layout":     "Centered form",
				"components": []interface{}{"Input: email", "Button: Sign in"},
				"transitions": []interface{}{
					map[string]interface{}{"trigger": "Sign in clicked", "to": "Dashboard"},
				},
			},
		},
	}
	got := sf.Render(filled)

	wantHas := []string{
		"# Screen Flow Spec",
		"Login flow",
		"Design Tokens",
		"#0070F3",
		"Screen 1: Sign In",
		"Let users authenticate",
		"Centered form",
		"Input: email",
		"Button: Sign in",
		"Sign in clicked",
		"Dashboard",
	}
	for _, sub := range wantHas {
		if !strings.Contains(got, sub) {
			t.Errorf("render missing %q\n--- full ---\n%s", sub, got)
		}
	}
}

func TestScreenFlowRenderMinimal(t *testing.T) {
	// Render must not panic on missing fields.
	sf := &ScreenFlow{}
	got := sf.Render(map[string]interface{}{})
	if !strings.Contains(got, "# Screen Flow Spec") {
		t.Errorf("expected heading even with empty input, got %q", got)
	}
}

func TestComponentSpecRender(t *testing.T) {
	cs := &ComponentSpec{}
	filled := map[string]interface{}{
		"name":    "Button",
		"summary": "A pressable element",
		"props": []interface{}{
			map[string]interface{}{
				"name":        "variant",
				"type":        "string",
				"required":    false,
				"description": "primary | secondary",
			},
		},
		"variants":      []interface{}{"primary", "secondary"},
		"states":        []interface{}{"default", "hover"},
		"a11y_notes":    []interface{}{"keyboard navigable"},
		"usage_example": "<Button variant='primary'>Click</Button>",
	}
	got := cs.Render(filled)

	wantHas := []string{
		"# Button",
		"pressable element",
		"## Props",
		"variant",
		"primary | secondary",
		"### Variants",
		"### States",
		"### Accessibility Notes",
		"keyboard navigable",
		"## Usage",
		"<Button variant='primary'>Click</Button>",
	}
	for _, sub := range wantHas {
		if !strings.Contains(got, sub) {
			t.Errorf("render missing %q", sub)
		}
	}
}

func TestUserJourneyRender(t *testing.T) {
	uj := &UserJourney{}
	filled := map[string]interface{}{
		"persona": map[string]interface{}{
			"name":    "Alex",
			"role":    "team lead",
			"context": "onboarding",
		},
		"goal": "Add a new hire to the team",
		"stages": []interface{}{
			map[string]interface{}{
				"name":          "Invitation",
				"actions":       []interface{}{"Open dashboard", "Click invite"},
				"thoughts":      []interface{}{"Is this the right place?"},
				"pain_points":   []interface{}{"Too many menus"},
				"opportunities": []interface{}{"Smart defaults"},
			},
		},
		"overall_pain":        "Friction in navigation",
		"overall_opportunity": "Proactive suggestions",
	}
	got := uj.Render(filled)

	wantHas := []string{
		"# User Journey",
		"Alex",
		"team lead",
		"onboarding",
		"Add a new hire",
		"Stage 1: Invitation",
		"Open dashboard",
		"Is this the right place?",
		"Too many menus",
		"Smart defaults",
		"Biggest Pain",
		"Friction in navigation",
		"Biggest Opportunity",
		"Proactive suggestions",
	}
	for _, sub := range wantHas {
		if !strings.Contains(got, sub) {
			t.Errorf("render missing %q", sub)
		}
	}
}

func TestFormatMetadata(t *testing.T) {
	formats := []Format{&ScreenFlow{}, &ComponentSpec{}, &UserJourney{}}
	for _, f := range formats {
		if f.Name() == "" {
			t.Errorf("%T has empty Name()", f)
		}
		if f.Description() == "" {
			t.Errorf("%T has empty Description()", f)
		}
		if f.SchemaHint() == "" {
			t.Errorf("%T has empty SchemaHint()", f)
		}
	}
}
