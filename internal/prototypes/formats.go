package prototypes

import (
	"fmt"
	"sort"
	"strings"
)

// Format is one supported design-md output shape. Each format owns its
// field schema (what Claude must produce) and its rendering (how to turn
// filled fields into the final markdown spec).
type Format interface {
	Name() string
	Description() string
	SchemaHint() string // tail of the Claude prompt describing required JSON fields
	Render(filled map[string]interface{}) string
}

// Registry picks a format by name.
type Registry struct {
	items    map[string]Format
	fallback Format
}

// NewRegistry wires the default set of formats.
func NewRegistry() *Registry {
	r := &Registry{items: make(map[string]Format)}
	r.Register(&ScreenFlow{})
	r.Register(&ComponentSpec{})
	r.Register(&UserJourney{})
	r.fallback = &ScreenFlow{}
	return r
}

// Register adds a format to the registry.
func (r *Registry) Register(f Format) { r.items[f.Name()] = f }

// Pick returns the named format, or the fallback.
func (r *Registry) Pick(name string) Format {
	if f, ok := r.items[name]; ok {
		return f
	}
	return r.fallback
}

// Names returns every registered format name in stable order.
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.items))
	for k := range r.items {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// --- Format: Screen Flow ---

// ScreenFlow renders a multi-screen UI flow with transitions — ideal for v0.
type ScreenFlow struct{}

func (s *ScreenFlow) Name() string { return "screen-flow" }

func (s *ScreenFlow) Description() string {
	return "Multi-screen UI flow with transitions. Good for v0/bolt/Figma Make."
}

func (s *ScreenFlow) SchemaHint() string {
	return `{
  "summary": "one paragraph describing the flow",
  "screens": [
    {
      "name": "Screen name",
      "purpose": "What this screen is for",
      "layout": "Rough layout description (hero, sidebar, etc.)",
      "components": ["Button: Sign in", "Input: email", "Input: password"],
      "transitions": [{"trigger": "Sign in clicked", "to": "Dashboard"}]
    }
  ],
  "design_tokens": {"primary_color": "#0070F3", "font": "Inter"}
}`
}

func (s *ScreenFlow) Render(filled map[string]interface{}) string {
	var b strings.Builder
	b.WriteString("# Screen Flow Spec\n\n")
	if summary := getString(filled, "summary"); summary != "" {
		b.WriteString(summary)
		b.WriteString("\n\n")
	}

	if tokens, ok := filled["design_tokens"].(map[string]interface{}); ok && len(tokens) > 0 {
		b.WriteString("## Design Tokens\n\n")
		for k, v := range tokens {
			b.WriteString(fmt.Sprintf("- **%s**: %v\n", k, v))
		}
		b.WriteString("\n")
	}

	screens, _ := filled["screens"].([]interface{})
	for i, raw := range screens {
		screen, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		b.WriteString(fmt.Sprintf("## Screen %d: %s\n\n", i+1, getString(screen, "name")))
		if p := getString(screen, "purpose"); p != "" {
			b.WriteString(fmt.Sprintf("**Purpose:** %s\n\n", p))
		}
		if l := getString(screen, "layout"); l != "" {
			b.WriteString(fmt.Sprintf("**Layout:** %s\n\n", l))
		}
		if comps, ok := screen["components"].([]interface{}); ok && len(comps) > 0 {
			b.WriteString("**Components:**\n\n")
			for _, c := range comps {
				b.WriteString(fmt.Sprintf("- %v\n", c))
			}
			b.WriteString("\n")
		}
		if trans, ok := screen["transitions"].([]interface{}); ok && len(trans) > 0 {
			b.WriteString("**Transitions:**\n\n")
			for _, t := range trans {
				if m, ok := t.(map[string]interface{}); ok {
					b.WriteString(fmt.Sprintf("- On `%s` → **%s**\n", getString(m, "trigger"), getString(m, "to")))
				}
			}
			b.WriteString("\n")
		}
	}

	return b.String()
}

// --- Format: Component Spec ---

// ComponentSpec describes a single UI component with props, variants, and states.
type ComponentSpec struct{}

func (c *ComponentSpec) Name() string { return "component-spec" }

func (c *ComponentSpec) Description() string {
	return "Single component with props, variants, and states. Good for design-system additions."
}

func (c *ComponentSpec) SchemaHint() string {
	return `{
  "name": "ComponentName",
  "summary": "what this component is",
  "props": [{"name": "variant", "type": "string", "required": false, "description": "primary | secondary"}],
  "variants": ["primary", "secondary", "ghost"],
  "states": ["default", "hover", "focus", "disabled"],
  "a11y_notes": ["ARIA roles", "keyboard navigation"],
  "usage_example": "<ComponentName variant='primary'>Click me</ComponentName>"
}`
}

func (c *ComponentSpec) Render(filled map[string]interface{}) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# %s\n\n", getString(filled, "name")))
	if s := getString(filled, "summary"); s != "" {
		b.WriteString(s)
		b.WriteString("\n\n")
	}

	if props, ok := filled["props"].([]interface{}); ok && len(props) > 0 {
		b.WriteString("## Props\n\n| Name | Type | Required | Description |\n|---|---|---|---|\n")
		for _, p := range props {
			if m, ok := p.(map[string]interface{}); ok {
				b.WriteString(fmt.Sprintf("| %s | %s | %v | %s |\n",
					getString(m, "name"),
					getString(m, "type"),
					m["required"],
					getString(m, "description")))
			}
		}
		b.WriteString("\n")
	}

	writeStringList(&b, "Variants", filled, "variants")
	writeStringList(&b, "States", filled, "states")
	writeStringList(&b, "Accessibility Notes", filled, "a11y_notes")

	if ex := getString(filled, "usage_example"); ex != "" {
		b.WriteString("## Usage\n\n```tsx\n")
		b.WriteString(ex)
		b.WriteString("\n```\n")
	}
	return b.String()
}

// --- Format: User Journey ---

// UserJourney narrates a walk-through from a persona's point of view.
type UserJourney struct{}

func (u *UserJourney) Name() string { return "user-journey" }

func (u *UserJourney) Description() string {
	return "Narrative walk-through from a persona POV with emotional beats."
}

func (u *UserJourney) SchemaHint() string {
	return `{
  "persona": {"name": "Alex", "role": "team lead", "context": "onboarding a new hire"},
  "goal": "What the persona is trying to accomplish",
  "stages": [
    {"name": "Stage name", "actions": ["What they do"], "thoughts": ["What they're thinking"], "pain_points": ["Frustrations"], "opportunities": ["Moments we can shine"]}
  ],
  "overall_pain": "biggest friction point",
  "overall_opportunity": "biggest opportunity"
}`
}

func (u *UserJourney) Render(filled map[string]interface{}) string {
	var b strings.Builder
	b.WriteString("# User Journey\n\n")

	if persona, ok := filled["persona"].(map[string]interface{}); ok {
		b.WriteString(fmt.Sprintf("**Persona:** %s · %s\n\n", getString(persona, "name"), getString(persona, "role")))
		if ctx := getString(persona, "context"); ctx != "" {
			b.WriteString(fmt.Sprintf("**Context:** %s\n\n", ctx))
		}
	}
	if goal := getString(filled, "goal"); goal != "" {
		b.WriteString(fmt.Sprintf("**Goal:** %s\n\n", goal))
	}

	stages, _ := filled["stages"].([]interface{})
	for i, raw := range stages {
		stage, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		b.WriteString(fmt.Sprintf("## Stage %d: %s\n\n", i+1, getString(stage, "name")))
		writeStringList(&b, "Actions", stage, "actions")
		writeStringList(&b, "Thoughts", stage, "thoughts")
		writeStringList(&b, "Pain points", stage, "pain_points")
		writeStringList(&b, "Opportunities", stage, "opportunities")
	}

	if p := getString(filled, "overall_pain"); p != "" {
		b.WriteString(fmt.Sprintf("## Biggest Pain\n\n%s\n\n", p))
	}
	if o := getString(filled, "overall_opportunity"); o != "" {
		b.WriteString(fmt.Sprintf("## Biggest Opportunity\n\n%s\n\n", o))
	}

	return b.String()
}

// --- helpers ---

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func writeStringList(b *strings.Builder, heading string, source map[string]interface{}, key string) {
	raw, ok := source[key].([]interface{})
	if !ok || len(raw) == 0 {
		return
	}
	b.WriteString(fmt.Sprintf("### %s\n\n", heading))
	for _, item := range raw {
		b.WriteString(fmt.Sprintf("- %v\n", item))
	}
	b.WriteString("\n")
}
