package projects

import (
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/kriswong/corticalstack/internal/vault"
)

func newTempStore(t *testing.T) *Store {
	t.Helper()
	dir, err := os.MkdirTemp("", "cortical-projects-*")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return New(vault.New(dir))
}

func TestCreateAndList(t *testing.T) {
	s := newTempStore(t)

	p, err := s.Create(CreateRequest{Name: "LicenseNinja", Description: "SaaS"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if p.ID != "licenseninja" {
		t.Errorf("id: got %q want %q", p.ID, "licenseninja")
	}
	if p.Status != StatusActive {
		t.Errorf("status: got %q want active", p.Status)
	}

	list := s.List()
	if len(list) != 1 {
		t.Fatalf("list length: got %d want 1", len(list))
	}
	if list[0].Name != "LicenseNinja" {
		t.Errorf("list[0].Name: got %q", list[0].Name)
	}
}

func TestRefreshDiscoversExisting(t *testing.T) {
	s := newTempStore(t)
	_, _ = s.Create(CreateRequest{Name: "Alpha"})

	// Fresh store over the same vault should find the existing project.
	fresh := New(s.vault)
	if err := fresh.Refresh(); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if fresh.Get("alpha") == nil {
		t.Errorf("expected alpha to be discovered")
	}
}

func TestCreateDuplicateFails(t *testing.T) {
	s := newTempStore(t)
	if _, err := s.Create(CreateRequest{Name: "Beta"}); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := s.Create(CreateRequest{Name: "Beta"})
	if err == nil {
		t.Errorf("expected duplicate create to fail")
	}
	if !errors.Is(err, ErrProjectExists) {
		t.Errorf("expected ErrProjectExists, got %v", err)
	}
}

// TestCreateIfMissingIdempotent covers MD-06 / MD-07: CreateIfMissing
// should return (existing, false, nil) on a second call with the same
// name, not error. Fan-out callers like SyncFromVault and EnsureExists
// rely on this to avoid log spam and false-negative "zero new projects"
// reports.
func TestCreateIfMissingIdempotent(t *testing.T) {
	s := newTempStore(t)

	p1, created1, err := s.CreateIfMissing(CreateRequest{Name: "Gamma"})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if !created1 {
		t.Errorf("first call should report created=true")
	}
	if p1 == nil {
		t.Fatal("first call returned nil project")
	}

	p2, created2, err := s.CreateIfMissing(CreateRequest{Name: "Gamma"})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if created2 {
		t.Errorf("second call should report created=false")
	}
	if p2 == nil || p2.ID != p1.ID {
		t.Errorf("second call should return existing project (p1=%v p2=%v)", p1, p2)
	}
}

// TestCreateIfMissingRaceSafe covers MD-06: concurrent CreateIfMissing
// for the same project must produce exactly one "created=true" outcome.
// Before the fix, the existence check and Create ran in separate lock
// scopes, so N concurrent goroutines could all pass the check and one
// would succeed while N-1 silently failed.
func TestCreateIfMissingRaceSafe(t *testing.T) {
	s := newTempStore(t)

	const goroutines = 10
	var (
		wg         sync.WaitGroup
		mu         sync.Mutex
		successes  int
		existingOk int
		otherErr   error
	)
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_, created, err := s.CreateIfMissing(CreateRequest{Name: "RacyProject"})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err != nil:
				if otherErr == nil {
					otherErr = err
				}
			case created:
				successes++
			default:
				existingOk++
			}
		}()
	}
	wg.Wait()

	if otherErr != nil {
		t.Fatalf("unexpected error: %v", otherErr)
	}
	if successes != 1 {
		t.Errorf("expected exactly 1 successful create, got %d", successes)
	}
	if existingOk != goroutines-1 {
		t.Errorf("expected %d existing-OK responses, got %d", goroutines-1, existingOk)
	}
}
