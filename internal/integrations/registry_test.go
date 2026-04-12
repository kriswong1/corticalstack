package integrations

import (
	"errors"
	"testing"
)

// fakeIntegration is a minimal Integration for registry tests.
type fakeIntegration struct {
	id         string
	name       string
	configured bool
	healthErr  error
}

func (f *fakeIntegration) ID() string          { return f.id }
func (f *fakeIntegration) Name() string        { return f.name }
func (f *fakeIntegration) Configured() bool    { return f.configured }
func (f *fakeIntegration) HealthCheck() error  { return f.healthErr }

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	fake := &fakeIntegration{id: "fake", name: "Fake"}

	if err := r.Register(fake); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got := r.Get("fake")
	if got == nil {
		t.Fatal("Get returned nil")
	}
	if got.ID() != "fake" {
		t.Errorf("ID = %q", got.ID())
	}
}

func TestRegistryRegisterDuplicate(t *testing.T) {
	r := NewRegistry()
	a := &fakeIntegration{id: "dup", name: "A"}
	b := &fakeIntegration{id: "dup", name: "B"}

	if err := r.Register(a); err != nil {
		t.Fatalf("first register: %v", err)
	}
	err := r.Register(b)
	if err == nil {
		t.Fatal("expected duplicate registration error, got nil")
	}
}

func TestRegistryGetMissing(t *testing.T) {
	r := NewRegistry()
	if got := r.Get("nope"); got != nil {
		t.Errorf("Get on empty registry = %v, want nil", got)
	}
}

func TestRegistryAll(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&fakeIntegration{id: "a", name: "A"})
	_ = r.Register(&fakeIntegration{id: "b", name: "B"})
	_ = r.Register(&fakeIntegration{id: "c", name: "C"})

	got := r.All()
	if len(got) != 3 {
		t.Errorf("All len = %d, want 3", len(got))
	}

	// Ensure each ID is present (order is map-iteration order, unstable).
	ids := make(map[string]bool)
	for _, i := range got {
		ids[i.ID()] = true
	}
	for _, want := range []string{"a", "b", "c"} {
		if !ids[want] {
			t.Errorf("All missing id %q", want)
		}
	}
}

func TestRegistryStatuses(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&fakeIntegration{id: "ok", name: "Configured One", configured: true})
	_ = r.Register(&fakeIntegration{id: "missing", name: "Unconfigured One", configured: false})

	got := r.Statuses()
	if len(got) != 2 {
		t.Fatalf("Statuses len = %d, want 2", len(got))
	}

	byID := make(map[string]Status)
	for _, s := range got {
		byID[s.ID] = s
	}

	if byID["ok"].Name != "Configured One" || !byID["ok"].Configured {
		t.Errorf("ok status wrong: %+v", byID["ok"])
	}
	if byID["missing"].Configured {
		t.Errorf("missing should not be configured: %+v", byID["missing"])
	}
}

// Sanity check that the HealthCheck is reachable via the interface.
func TestFakeIntegrationHealthError(t *testing.T) {
	f := &fakeIntegration{healthErr: errors.New("down")}
	if err := f.HealthCheck(); err == nil || err.Error() != "down" {
		t.Errorf("HealthCheck = %v", err)
	}
}
