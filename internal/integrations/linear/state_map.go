package linear

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/kriswong/corticalstack/internal/actions"
)

// stateMapPath is where the per-team Action↔Linear workflow state
// mapping is stored. Sibling of the actions index so a single
// `vault/.cortical/` directory holds all integration state.
const stateMapPath = ".cortical/linear-state-map.json"

// stateMapFile is the on-disk shape: a per-team mapping of action
// status → linear workflow state id. Keyed by team key (e.g. "BCN")
// so multi-team setups can keep distinct mappings.
type stateMapFile struct {
	Teams map[string]map[string]string `json:"teams"`
}

// stateMapLoader caches the loaded file in-process so repeated sync
// calls don't re-read disk. Disk → memory is one-shot per process;
// callers must restart cortical to pick up hand-edits.
type stateMapLoader struct {
	mu     sync.Mutex
	loaded *stateMapFile
}

var globalStateMap = &stateMapLoader{}

// loadOrBootstrapStateMap returns a {action.Status → linear stateId}
// map for the given team. If the on-disk file is missing or has no
// entry for this team, a default mapping is computed from the team's
// workflow states (matched by lowercase name) and persisted.
//
// L7: takes an explicit client + teamKey so the caller can route the
// bootstrap call through whichever workspace's API key applies.
func (o *Orchestrator) loadOrBootstrapStateMap(ctx context.Context, client *Client, teamKey, teamID string) (map[actions.Status]string, error) {
	teamKey = strings.TrimSpace(teamKey)
	if teamKey == "" {
		return nil, fmt.Errorf("no team key configured")
	}
	if o.Stores.Actions == nil {
		return nil, fmt.Errorf("actions store not wired")
	}

	globalStateMap.mu.Lock()
	defer globalStateMap.mu.Unlock()

	vaultPath := o.Stores.Actions.VaultPath()
	abs := filepath.Join(vaultPath, stateMapPath)

	if globalStateMap.loaded == nil {
		globalStateMap.loaded = &stateMapFile{Teams: map[string]map[string]string{}}
		if data, err := os.ReadFile(abs); err == nil {
			_ = json.Unmarshal(data, globalStateMap.loaded)
			if globalStateMap.loaded.Teams == nil {
				globalStateMap.loaded.Teams = map[string]map[string]string{}
			}
		}
	}

	teamMap, ok := globalStateMap.loaded.Teams[teamKey]
	if !ok || len(teamMap) == 0 {
		// Bootstrap from Linear's workflow states.
		states, err := client.ListWorkflowStates(ctx, teamID)
		if err != nil {
			return nil, fmt.Errorf("bootstrap state map: %w", err)
		}
		teamMap = bootstrapStateMap(states)
		globalStateMap.loaded.Teams[teamKey] = teamMap
		if err := persistStateMap(abs, globalStateMap.loaded); err != nil {
			// Non-fatal — sync still works, just won't be persisted.
			// Log via fmt.Errorf wrap so callers can decide.
			_ = err
		}
	}

	out := make(map[actions.Status]string, len(teamMap))
	for status, stateID := range teamMap {
		out[actions.Status(status)] = stateID
	}
	return out, nil
}

// bootstrapStateMap computes a sensible default mapping by name+type.
// Falls back gracefully when a team's workflow doesn't have an exact
// match for one of CorticalStack's statuses.
func bootstrapStateMap(states []WorkflowState) map[string]string {
	byNameLower := map[string]string{}
	byType := map[string][]string{}
	for _, s := range states {
		byNameLower[strings.ToLower(s.Name)] = s.ID
		byType[s.Type] = append(byType[s.Type], s.ID)
	}
	pickByType := func(t string) string {
		if ids, ok := byType[t]; ok && len(ids) > 0 {
			return ids[0]
		}
		return ""
	}
	pickByName := func(names ...string) string {
		for _, n := range names {
			if id, ok := byNameLower[strings.ToLower(n)]; ok {
				return id
			}
		}
		return ""
	}

	m := map[string]string{}
	if id := pickByName("Backlog"); id != "" {
		m[string(actions.StatusInbox)] = id
	} else if id := pickByType("backlog"); id != "" {
		m[string(actions.StatusInbox)] = id
	}
	if id := pickByName("Todo", "To Do", "Up Next", "Next"); id != "" {
		m[string(actions.StatusNext)] = id
	} else if id := pickByType("unstarted"); id != "" {
		m[string(actions.StatusNext)] = id
	}
	if id := pickByName("In Progress", "Doing", "Started"); id != "" {
		m[string(actions.StatusDoing)] = id
	} else if id := pickByType("started"); id != "" {
		m[string(actions.StatusDoing)] = id
	}
	if id := pickByName("Blocked", "Waiting", "On Hold"); id != "" {
		m[string(actions.StatusWaiting)] = id
	} else if id := pickByType("started"); id != "" {
		m[string(actions.StatusWaiting)] = id
	}
	if id := pickByName("Someday", "Backlog"); id != "" {
		m[string(actions.StatusSomeday)] = id
	} else if id := pickByType("backlog"); id != "" {
		m[string(actions.StatusSomeday)] = id
	}
	if id := pickByName("Deferred", "Triage"); id != "" {
		m[string(actions.StatusDeferred)] = id
	} else if id := pickByType("backlog"); id != "" {
		m[string(actions.StatusDeferred)] = id
	}
	if id := pickByName("Done", "Completed"); id != "" {
		m[string(actions.StatusDone)] = id
	} else if id := pickByType("completed"); id != "" {
		m[string(actions.StatusDone)] = id
	}
	if id := pickByName("Cancelled", "Canceled"); id != "" {
		m[string(actions.StatusCancelled)] = id
	} else if id := pickByType("canceled"); id != "" {
		m[string(actions.StatusCancelled)] = id
	}
	return m
}

func persistStateMap(path string, file *stateMapFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
