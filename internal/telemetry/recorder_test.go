package telemetry

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/kriswong/corticalstack/internal/agent"
)

func TestJSONLRecorderRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.jsonl")
	rec, err := NewJSONLRecorder(path)
	if err != nil {
		t.Fatalf("NewJSONLRecorder: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	want := agent.Invocation{
		Timestamp:           now,
		Model:               "claude-sonnet-4-5",
		SessionID:           "sess-roundtrip",
		InputTokens:         42,
		OutputTokens:        17,
		CacheCreationTokens: 100,
		CacheReadTokens:     200,
		CostUSD:             0.0123,
		DurationMS:          1500,
		DurationAPIMS:       1200,
		NumTurns:            1,
		Subtype:             "success",
		WorkingDir:          "/tmp",
		MaxTurns:            1,
		CallerHint:          "test.roundtrip",
		PromptLen:           500,
		ResultLen:           250,
	}
	rec.Record(want)
	if err := rec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var got agent.Invocation
	if err := json.Unmarshal(data[:len(data)-1], &got); err != nil { // strip trailing \n
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.SessionID != want.SessionID {
		t.Errorf("session_id = %q, want %q", got.SessionID, want.SessionID)
	}
	if got.InputTokens != want.InputTokens {
		t.Errorf("input_tokens = %d", got.InputTokens)
	}
	if got.CostUSD != want.CostUSD {
		t.Errorf("cost_usd = %v", got.CostUSD)
	}
	if got.CallerHint != want.CallerHint {
		t.Errorf("caller_hint = %q", got.CallerHint)
	}
	if !got.Timestamp.Equal(want.Timestamp) {
		t.Errorf("timestamp = %v, want %v", got.Timestamp, want.Timestamp)
	}
}

func TestJSONLRecorderCreatesParentDir(t *testing.T) {
	// Path under a non-existent subdirectory — recorder must mkdir it.
	path := filepath.Join(t.TempDir(), "nested", ".cortical", "usage.jsonl")
	rec, err := NewJSONLRecorder(path)
	if err != nil {
		t.Fatalf("NewJSONLRecorder: %v", err)
	}
	defer rec.Close()

	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Errorf("parent dir not created: %v", err)
	}
}

func TestJSONLRecorderEmptyPathReturnsError(t *testing.T) {
	if _, err := NewJSONLRecorder(""); err == nil {
		t.Error("expected error for empty path")
	}
}

// 100 goroutines hammer Record concurrently. Every line in the file
// must be valid JSON (proves single-Write atomicity and mutex coverage).
// Full json decode catches partial-write interleaving that wc -l won't.
func TestJSONLRecorderConcurrentWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.jsonl")
	rec, err := NewJSONLRecorder(path)
	if err != nil {
		t.Fatalf("NewJSONLRecorder: %v", err)
	}

	const n = 100
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			rec.Record(agent.Invocation{
				Timestamp: time.Now(),
				Model:     "m",
				SessionID: "sess-" + strconv.Itoa(i),
				CostUSD:   float64(i) * 0.01,
			})
		}(i)
	}
	wg.Wait()
	if err := rec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	seen := make(map[string]bool, n)
	var count int
	for scanner.Scan() {
		count++
		var inv agent.Invocation
		if err := json.Unmarshal(scanner.Bytes(), &inv); err != nil {
			t.Fatalf("line %d not valid JSON: %v\nline: %s", count, err, scanner.Text())
		}
		if seen[inv.SessionID] {
			t.Errorf("duplicate session_id %q", inv.SessionID)
		}
		seen[inv.SessionID] = true
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if count != n {
		t.Errorf("line count = %d, want %d", count, n)
	}
	if len(seen) != n {
		t.Errorf("unique session ids = %d, want %d", len(seen), n)
	}
}

func TestJSONLRecorderDoubleCloseSafe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.jsonl")
	rec, err := NewJSONLRecorder(path)
	if err != nil {
		t.Fatalf("NewJSONLRecorder: %v", err)
	}
	if err := rec.Close(); err != nil {
		t.Errorf("first close: %v", err)
	}
	if err := rec.Close(); err != nil {
		t.Errorf("second close: %v", err)
	}
}
