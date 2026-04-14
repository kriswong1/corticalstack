package actions

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kriswong/corticalstack/internal/vault"
)

func newTempStore(t *testing.T) *Store {
	t.Helper()
	dir, err := os.MkdirTemp("", "cortical-actions-*")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	s := New(vault.New(dir))
	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := s.EnsureCentralFile(); err != nil {
		t.Fatalf("ensure central: %v", err)
	}
	return s
}

func TestUpsertAndSync(t *testing.T) {
	s := newTempStore(t)

	a, err := s.Upsert(&Action{
		Owner:       "Kris",
		Description: "Write tests",
		Status:      StatusPending,
		SourceNote:  "notes/2026-04-11_hello.md",
		ProjectIDs:  []string{"corticalstack"},
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if a.ID == "" {
		t.Fatal("expected non-empty id after upsert")
	}
	if err := s.Sync(a); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Central file should contain the line
	central := filepath.Join(s.VaultPath(), s.CentralFilePath())
	data, _ := os.ReadFile(central)
	if !strings.Contains(string(data), "id:"+a.ID) {
		t.Errorf("central file missing id marker; content:\n%s", data)
	}
	// Project file should contain the line
	project := filepath.Join(s.VaultPath(), s.ProjectFilePath("corticalstack"))
	data, _ = os.ReadFile(project)
	if !strings.Contains(string(data), "id:"+a.ID) {
		t.Errorf("project file missing id marker; content:\n%s", data)
	}
}

func TestSetStatusPropagates(t *testing.T) {
	s := newTempStore(t)

	a, _ := s.Upsert(&Action{
		Owner:       "Kris",
		Description: "Move status",
		Status:      StatusPending,
		SourceNote:  "notes/move.md",
		ProjectIDs:  []string{"p1"},
	})
	if err := s.Sync(a); err != nil {
		t.Fatalf("initial sync: %v", err)
	}

	updated, err := s.SetStatus(a.ID, StatusDoing)
	if err != nil {
		t.Fatalf("set status: %v", err)
	}
	if err := s.Sync(updated); err != nil {
		t.Fatalf("sync after status: %v", err)
	}

	// Central should now contain #status/doing
	central := filepath.Join(s.VaultPath(), s.CentralFilePath())
	data, _ := os.ReadFile(central)
	if !strings.Contains(string(data), "#status/doing") {
		t.Errorf("central file missing new status; content:\n%s", data)
	}
}

func TestCountByStatus(t *testing.T) {
	s := newTempStore(t)
	_, _ = s.Upsert(&Action{Owner: "a", Description: "1", Status: StatusPending}) // migrated to inbox
	_, _ = s.Upsert(&Action{Owner: "b", Description: "2", Status: StatusDone})
	_, _ = s.Upsert(&Action{Owner: "c", Description: "3", Status: StatusDone})

	counts := s.CountByStatus()
	if counts[StatusInbox] != 1 {
		t.Errorf("inbox: got %d want 1", counts[StatusInbox])
	}
	if counts[StatusDone] != 2 {
		t.Errorf("done: got %d want 2", counts[StatusDone])
	}
}

// TestUpsertDoesNotBumpUpdatedOnNoOp covers MD-02: an idempotent re-upsert
// of an action with identical field values should NOT bump Updated, so
// dashboard stalled-item detection isn't clobbered by re-ingest churn.
func TestUpsertDoesNotBumpUpdatedOnNoOp(t *testing.T) {
	s := newTempStore(t)

	first, err := s.Upsert(&Action{
		ID:          "no-op-id",
		Owner:       "Kris",
		Description: "Stable description",
		Status:      StatusInbox,
		Priority:    PriorityMedium,
		Effort:      EffortS,
		ProjectIDs:  []string{"proj-a"},
	})
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	t1 := first.Updated
	if t1.IsZero() {
		t.Fatal("expected non-zero Updated after insert")
	}

	// Sleep briefly so a bad implementation (bumping Updated) would
	// produce a visibly-different timestamp.
	time.Sleep(5 * time.Millisecond)

	// Re-upsert with identical fields. Even though time.Now() will have
	// advanced, Upsert must detect the no-op and preserve the original
	// Updated.
	second, err := s.Upsert(&Action{
		ID:          "no-op-id",
		Owner:       "Kris",
		Description: "Stable description",
		Status:      StatusInbox,
		Priority:    PriorityMedium,
		Effort:      EffortS,
		ProjectIDs:  []string{"proj-a"},
	})
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if !second.Updated.Equal(t1) {
		t.Errorf("Updated changed on no-op upsert: t1=%v t2=%v", t1, second.Updated)
	}
}

// TestUpsertBumpsUpdatedOnFieldChange covers the other half of MD-02:
// a real field change MUST still bump Updated.
func TestUpsertBumpsUpdatedOnFieldChange(t *testing.T) {
	s := newTempStore(t)

	first, err := s.Upsert(&Action{
		ID:          "change-id",
		Owner:       "Kris",
		Description: "First pass",
		Status:      StatusInbox,
		Priority:    PriorityMedium,
		Effort:      EffortS,
	})
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	t1 := first.Updated

	time.Sleep(5 * time.Millisecond)

	// Change Priority — should bump Updated.
	second, err := s.Upsert(&Action{
		ID:          "change-id",
		Owner:       "Kris",
		Description: "First pass",
		Status:      StatusInbox,
		Priority:    PriorityHigh, // changed
		Effort:      EffortS,
	})
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if !second.Updated.After(t1) {
		t.Errorf("expected Updated to advance on field change; t1=%v t2=%v", t1, second.Updated)
	}
	if second.Priority != PriorityHigh {
		t.Errorf("Priority not applied: got %v", second.Priority)
	}
}

// TestUpsertPreservesPointerIdentity covers MD-03: Upsert must mutate the
// existing *Action in place rather than replace the map entry, so callers
// that kept a pointer from an earlier Get() see the updated state.
func TestUpsertPreservesPointerIdentity(t *testing.T) {
	s := newTempStore(t)

	_, err := s.Upsert(&Action{
		ID:          "ptr-id",
		Owner:       "Kris",
		Description: "Original",
		Status:      StatusInbox,
	})
	if err != nil {
		t.Fatalf("initial upsert: %v", err)
	}

	// Capture the pointer BEFORE the second upsert.
	before := s.Get("ptr-id")
	if before == nil {
		t.Fatal("Get returned nil after insert")
	}
	if before.Description != "Original" {
		t.Fatalf("unexpected initial description: %q", before.Description)
	}

	// Upsert a change.
	updated, err := s.Upsert(&Action{
		ID:          "ptr-id",
		Owner:       "Kris",
		Description: "Revised", // changed
		Status:      StatusInbox,
	})
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	// The previously-held pointer must now reflect the new state.
	if before.Description != "Revised" {
		t.Errorf("held pointer did not observe mutation: got %q, want %q",
			before.Description, "Revised")
	}

	// And a fresh Get must return THE SAME pointer (identity-preserving).
	after := s.Get("ptr-id")
	if after != before {
		t.Errorf("pointer identity not preserved: before=%p after=%p", before, after)
	}
	if updated != before {
		t.Errorf("Upsert returned a different pointer than the stored one: "+
			"returned=%p stored=%p", updated, before)
	}
}

// TestUpsertFlushFailureDoesNotMutateMemory covers MD-10: if flushLocked
// fails (e.g., disk permission error), Upsert must NOT leave the new
// state in memory — otherwise List() returns an unpersisted entry that
// vanishes on restart.
func TestUpsertFlushFailureDoesNotMutateMemory(t *testing.T) {
	s := newTempStore(t)

	// Seed one real action so we can exercise the update path too.
	orig, err := s.Upsert(&Action{
		ID:          "flush-id",
		Owner:       "Kris",
		Description: "Original",
		Status:      StatusInbox,
		Priority:    PriorityMedium,
	})
	if err != nil {
		t.Fatalf("seed upsert: %v", err)
	}
	origDescription := orig.Description
	origUpdated := orig.Updated

	// Point the store at a non-existent parent directory so flushLocked
	// fails on os.MkdirAll / os.WriteFile. Easiest repro: remove the
	// entire vault dir out from under it.
	if err := os.RemoveAll(s.vault.Path()); err != nil {
		t.Fatalf("removing vault dir: %v", err)
	}
	// Re-create the vault dir as a regular FILE so MkdirAll inside
	// .cortical/ fails. This is the most reliable cross-platform way to
	// induce a flush failure.
	if err := os.WriteFile(s.vault.Path(), []byte("not a dir"), 0o600); err != nil {
		t.Fatalf("replacing vault dir with a file: %v", err)
	}
	// Cleanup: remove the file we just created so the test harness
	// cleanup tempdir removal doesn't trip on it.
	t.Cleanup(func() { _ = os.Remove(s.vault.Path()) })

	// Update path: try to change the description. flushLocked must fail
	// and the in-memory action must retain Original / origUpdated.
	_, err = s.Upsert(&Action{
		ID:          "flush-id",
		Owner:       "Kris",
		Description: "Should not persist",
		Status:      StatusInbox,
		Priority:    PriorityMedium,
	})
	if err == nil {
		t.Fatal("expected flush failure on update path; got nil")
	}
	got := s.Get("flush-id")
	if got == nil {
		t.Fatal("Get returned nil after failed update — memory corrupted")
	}
	if got.Description != origDescription {
		t.Errorf("update path leaked unpersisted field: got %q want %q",
			got.Description, origDescription)
	}
	if !got.Updated.Equal(origUpdated) {
		t.Errorf("update path leaked new Updated: got %v want %v",
			got.Updated, origUpdated)
	}

	// Insert path: a brand new ID whose flush also fails must NOT appear
	// in the in-memory map.
	_, err = s.Upsert(&Action{
		ID:          "new-flush-id",
		Owner:       "Kris",
		Description: "New row",
		Status:      StatusInbox,
		Priority:    PriorityMedium,
	})
	if err == nil {
		t.Fatal("expected flush failure on insert path; got nil")
	}
	if s.Get("new-flush-id") != nil {
		t.Error("insert path leaked unpersisted row into memory")
	}
}

// TestSetStatusWithLimitIsAtomic covers HI-02: the WIP-limit check and
// status mutation must happen under a single critical section so that N
// concurrent transitions into StatusDoing cannot overshoot the cap.
func TestSetStatusWithLimitIsAtomic(t *testing.T) {
	s := newTempStore(t)

	const numActions = 10
	const wipLimit = 2

	// Seed numActions actions all in StatusInbox.
	ids := make([]string, numActions)
	for i := 0; i < numActions; i++ {
		a, err := s.Upsert(&Action{
			Owner:       "Kris",
			Description: "task",
			Status:      StatusInbox,
		})
		if err != nil {
			t.Fatalf("seed upsert %d: %v", i, err)
		}
		ids[i] = a.ID
	}

	// Fan out: every goroutine tries to move a distinct action into
	// StatusDoing under the same wipLimit. Only `wipLimit` should win.
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		wins     int
		wipFails int
		otherErr error
	)
	wg.Add(numActions)
	for i := 0; i < numActions; i++ {
		go func(id string) {
			defer wg.Done()
			_, err := s.SetStatusWithLimit(id, StatusDoing, wipLimit)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				wins++
			case errors.Is(err, ErrWIPLimit):
				wipFails++
			default:
				if otherErr == nil {
					otherErr = err
				}
			}
		}(ids[i])
	}
	wg.Wait()

	if otherErr != nil {
		t.Fatalf("unexpected error: %v", otherErr)
	}
	if wins != wipLimit {
		t.Errorf("wins = %d, want %d", wins, wipLimit)
	}
	if wipFails != numActions-wipLimit {
		t.Errorf("wipFails = %d, want %d", wipFails, numActions-wipLimit)
	}

	// Sanity check: the store's final state must agree with the win
	// count (no overshoot, no undercount).
	counts := s.CountByStatus()
	if counts[StatusDoing] != wipLimit {
		t.Errorf("final StatusDoing count = %d, want %d",
			counts[StatusDoing], wipLimit)
	}
}

// TestSyncIsSerializedAgainstConcurrentWrites covers HI-03: N goroutines
// calling Sync on distinct actions that all live in the same project's
// ACTION-ITEMS.md must all end up in the file (no lost writes).
func TestSyncIsSerializedAgainstConcurrentWrites(t *testing.T) {
	s := newTempStore(t)

	const numActions = 20
	const project = "shared-proj"

	// Seed the actions in the store first (serially) so that the
	// concurrent phase tests Sync and only Sync.
	seeded := make([]*Action, 0, numActions)
	for i := 0; i < numActions; i++ {
		a, err := s.Upsert(&Action{
			Owner:       "Kris",
			Description: "task-" + randSuffix(i),
			Status:      StatusInbox,
			ProjectIDs:  []string{project},
		})
		if err != nil {
			t.Fatalf("seed upsert %d: %v", i, err)
		}
		seeded = append(seeded, a)
	}

	var wg sync.WaitGroup
	wg.Add(numActions)
	errCh := make(chan error, numActions)
	for i := 0; i < numActions; i++ {
		go func(a *Action) {
			defer wg.Done()
			if err := s.Sync(a); err != nil {
				errCh <- err
			}
		}(seeded[i])
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("sync error: %v", err)
	}

	// Read the project file and assert every action's ID marker is
	// present. Before HI-03 was fixed, later writers would clobber
	// earlier writers and the file would contain only the last few.
	projFile := filepath.Join(s.VaultPath(), s.ProjectFilePath(project))
	data, err := os.ReadFile(projFile)
	if err != nil {
		t.Fatalf("reading project file: %v", err)
	}
	content := string(data)
	for _, a := range seeded {
		marker := "<!-- id:" + a.ID + " -->"
		if !strings.Contains(content, marker) {
			t.Errorf("project file missing action %s (lost write)", a.ID)
		}
	}

	// Also assert the central file contains all of them.
	centralFile := filepath.Join(s.VaultPath(), s.CentralFilePath())
	centralData, err := os.ReadFile(centralFile)
	if err != nil {
		t.Fatalf("reading central file: %v", err)
	}
	centralContent := string(centralData)
	for _, a := range seeded {
		marker := "<!-- id:" + a.ID + " -->"
		if !strings.Contains(centralContent, marker) {
			t.Errorf("central file missing action %s (lost write)", a.ID)
		}
	}
}

// randSuffix produces a stable short string for test task descriptions.
func randSuffix(i int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz"
	return string(alphabet[i%len(alphabet)])
}
