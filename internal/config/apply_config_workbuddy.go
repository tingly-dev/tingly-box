package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// WorkBuddy config application: merge tingly-managed model entries into the
// models.json of every installed WorkBuddy edition while preserving everything
// the user put there. The desktop app keeps its custom models in
// ~/.workbuddy/models.json (China edition) or ~/.workbuddy-ai/models.json
// (international edition), as a top-level JSON array; see
// .design/workbuddy-agent.md.

// workBuddyEditions maps each WorkBuddy distribution to its config folder under
// the home directory; both can be installed side by side.
var workBuddyEditions = []struct{ name, dir string }{
	{name: "cn", dir: ".workbuddy"},
	{name: "intl", dir: ".workbuddy-ai"},
}

// WorkBuddyTarget is one models.json Auto Config manages.
type WorkBuddyTarget struct {
	Edition     string
	Path        string // absolute
	DisplayPath string // "~/..." form shown to the user
}

// WorkBuddyTargets returns the models.json of every installed edition (its
// config folder exists) and, separately, every edition's file, for messages.
func WorkBuddyTargets() (installed, all []WorkBuddyTarget, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	if home == "" {
		return nil, nil, fmt.Errorf("home directory resolved to empty path")
	}
	for _, e := range workBuddyEditions {
		t := WorkBuddyTarget{
			Edition:     e.name,
			Path:        filepath.Join(home, e.dir, "models.json"),
			DisplayPath: "~/" + e.dir + "/models.json",
		}
		all = append(all, t)
		if info, statErr := os.Stat(filepath.Join(home, e.dir)); statErr == nil && info.IsDir() {
			installed = append(installed, t)
		}
	}
	return installed, all, nil
}

// WorkBuddyDisplayPaths joins the targets' display paths for messages.
func WorkBuddyDisplayPaths(targets []WorkBuddyTarget) string {
	paths := make([]string, 0, len(targets))
	for _, t := range targets {
		paths = append(paths, t.DisplayPath)
	}
	return strings.Join(paths, ", ")
}

// WorkBuddyNotInstalledMessage is reported when no edition's folder exists.
func WorkBuddyNotInstalledMessage(all []WorkBuddyTarget) string {
	dirs := make([]string, 0, len(all))
	for _, t := range all {
		dirs = append(dirs, filepath.ToSlash(filepath.Dir(t.DisplayPath)))
	}
	return fmt.Sprintf("WorkBuddy not found: none of %s exists. Open WorkBuddy once so it creates its config folder, then try again.",
		strings.Join(dirs, ", "))
}

// workBuddyEndpointMarker tags the entries tingly-box owns: their `url` points at
// this gateway's WorkBuddy scenario. The schema has no free-form metadata field,
// so ownership is derived from the URL instead of an extra key WorkBuddy might
// reject.
const workBuddyEndpointMarker = "/tingly/workbuddy"

// workBuddyChatPath is appended to the scenario base URL; WorkBuddy wants the
// complete chat-completions endpoint.
const workBuddyChatPath = "/v1/chat/completions"

// workBuddyVendor is the group the entries show under in WorkBuddy's model picker.
const workBuddyVendor = "Tingly Box"

// BuildWorkBuddyModelEntries renders one models.json entry per model id.
// baseURL is the scenario base (".../tingly/workbuddy"). Capability flags are
// conservative — tool calling on, images and reasoning off — since WorkBuddy
// reports call errors for flags the routed model does not support.
func BuildWorkBuddyModelEntries(baseURL, apiKey string, models []string) []map[string]interface{} {
	url := strings.TrimRight(baseURL, "/") + workBuddyChatPath
	entries := make([]map[string]interface{}, 0, len(models))
	for _, id := range models {
		entries = append(entries, map[string]interface{}{
			"id":                id,
			"name":              id,
			"vendor":            workBuddyVendor,
			"url":               url,
			"apiKey":            apiKey,
			"supportsToolCall":  true,
			"supportsImages":    false,
			"supportsReasoning": false,
		})
	}
	return entries
}

func isWorkBuddyManaged(entry interface{}) bool {
	m, ok := entry.(map[string]interface{})
	if !ok {
		return false
	}
	url, _ := m["url"].(string)
	return strings.Contains(url, workBuddyEndpointMarker+"/")
}

func entryID(entry interface{}) string {
	m, ok := entry.(map[string]interface{})
	if !ok {
		return ""
	}
	id, _ := m["id"].(string)
	return id
}

// WorkBuddyApplyResult extends ApplyResult with the model ids that could not be
// written because the user already has an unrelated entry with the same id, and
// the per-file outcome when several editions are installed.
type WorkBuddyApplyResult struct {
	ApplyResult
	Skipped []string              `json:"skipped,omitempty"`
	Files   []WorkBuddyFileResult `json:"files,omitempty"`
}

// WorkBuddyFileResult is the outcome for one edition's models.json.
type WorkBuddyFileResult struct {
	Edition    string   `json:"edition"`
	Path       string   `json:"path"`
	Success    bool     `json:"success"`
	Created    bool     `json:"created,omitempty"`
	Updated    bool     `json:"updated,omitempty"`
	BackupPath string   `json:"backupPath,omitempty"`
	Skipped    []string `json:"skipped,omitempty"`
	Message    string   `json:"message"`
}

// mergeWorkBuddyEntries replaces tingly-managed entries in models with desired,
// never touching other entries. A desired id that collides with a user-owned
// entry is skipped (returned) rather than overwriting it.
func mergeWorkBuddyEntries(models []interface{}, desired []map[string]interface{}) (merged []interface{}, written, oldManaged map[string]struct{}, skipped []string) {
	merged = []interface{}{}
	userIDs := map[string]struct{}{}
	oldManaged = map[string]struct{}{}
	for _, e := range models {
		if isWorkBuddyManaged(e) {
			oldManaged[entryID(e)] = struct{}{}
			continue
		}
		merged = append(merged, e)
		if id := entryID(e); id != "" {
			userIDs[id] = struct{}{}
		}
	}

	written = map[string]struct{}{}
	for _, d := range desired {
		id, _ := d["id"].(string)
		if _, clash := userIDs[id]; clash {
			skipped = append(skipped, id)
			continue
		}
		merged = append(merged, d)
		written[id] = struct{}{}
	}
	return merged, written, oldManaged, skipped
}

// mergeWorkBuddyModels merges into the object form ({"models": [...]}). When an
// availableModels whitelist exists it is kept consistent: stale managed ids are
// dropped and the written ids added, otherwise our models would be filtered out.
func mergeWorkBuddyModels(existing map[string]interface{}, desired []map[string]interface{}) (skipped []string) {
	models, _ := existing["models"].([]interface{})
	merged, written, oldManaged, skipped := mergeWorkBuddyEntries(models, desired)
	existing["models"] = merged

	if avail, ok := existing["availableModels"].([]interface{}); ok && len(avail) > 0 {
		var next []interface{}
		have := map[string]struct{}{}
		for _, a := range avail {
			if s, ok := a.(string); ok {
				if _, stale := oldManaged[s]; stale {
					if _, still := written[s]; !still {
						continue
					}
				}
				have[s] = struct{}{}
			}
			next = append(next, a)
		}
		ids := make([]string, 0, len(written))
		for id := range written {
			ids = append(ids, id)
		}
		slices.Sort(ids)
		for _, id := range ids {
			if _, ok := have[id]; !ok {
				next = append(next, id)
			}
		}
		existing["availableModels"] = next
	}
	return skipped
}

// ApplyWorkBuddyConfig merges the tingly-managed entries into the models.json
// of every installed WorkBuddy edition. Each file is handled independently, so
// a broken file in one edition does not block the other; the overall result
// succeeds only when every file did.
func ApplyWorkBuddyConfig(baseURL, apiKey string, models []string) (*WorkBuddyApplyResult, error) {
	installed, all, err := WorkBuddyTargets()
	if err != nil {
		return nil, err
	}
	result := &WorkBuddyApplyResult{}
	if len(installed) == 0 {
		result.Message = WorkBuddyNotInstalledMessage(all)
		return result, nil
	}

	result.Success = true
	messages := make([]string, 0, len(installed))
	seenSkipped := map[string]struct{}{}
	for _, t := range installed {
		f := applyWorkBuddyConfigAt(t.Path, t.DisplayPath, baseURL, apiKey, models)
		f.Edition = t.Edition
		result.Files = append(result.Files, f)
		result.Success = result.Success && f.Success
		result.Created = result.Created || f.Created
		result.Updated = result.Updated || f.Updated
		if result.BackupPath == "" {
			result.BackupPath = f.BackupPath
		}
		for _, id := range f.Skipped {
			if _, dup := seenSkipped[id]; !dup {
				seenSkipped[id] = struct{}{}
				result.Skipped = append(result.Skipped, id)
			}
		}
		messages = append(messages, f.Message)
	}
	result.Message = strings.Join(messages, "; ")
	return result, nil
}

// applyWorkBuddyConfigAt merges into one models.json. It accepts both the
// desktop app's top-level array and the documented {"models": [...]} object,
// writes back the same shape (array for a new file), refuses (touching
// nothing) on anything else, backs up before overwriting, and writes
// atomically with 0600 permissions because the file holds API keys.
func applyWorkBuddyConfigAt(targetPath, displayPath, baseURL, apiKey string, models []string) WorkBuddyFileResult {
	result := WorkBuddyFileResult{Path: displayPath}
	fail := func(format string, a ...interface{}) WorkBuddyFileResult {
		result.Success = false
		result.Message = fmt.Sprintf(format, a...)
		return result
	}

	var doc interface{} = []interface{}{}
	mode := os.FileMode(0o600)
	info, statErr := os.Stat(targetPath)
	if statErr == nil {
		data, err := os.ReadFile(targetPath)
		if err != nil {
			return fail("Failed to read %s: %v", displayPath, err)
		}
		data = trimUTF8BOM(data)
		if len(strings.TrimSpace(string(data))) > 0 {
			var parsed interface{}
			if err := json.Unmarshal(data, &parsed); err != nil {
				return fail("%s is not valid JSON (%v); left untouched", displayPath, err)
			}
			switch parsed.(type) {
			case []interface{}, map[string]interface{}:
				doc = parsed
			default:
				return fail("%s is not a JSON array or object; left untouched", displayPath)
			}
		}
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(statErr) {
		return fail("Failed to stat %s: %v", displayPath, statErr)
	}

	desired := BuildWorkBuddyModelEntries(baseURL, apiKey, models)
	switch d := doc.(type) {
	case []interface{}:
		var merged []interface{}
		merged, _, _, result.Skipped = mergeWorkBuddyEntries(d, desired)
		doc = merged
	case map[string]interface{}:
		result.Skipped = mergeWorkBuddyModels(d, desired)
	}
	// Every id is taken by one of the user's own models: writing would only
	// drop our stale entries and add nothing, so refuse before touching the file.
	if len(models) > 0 && len(result.Skipped) == len(models) {
		return fail("Nothing written: %s already has models with ids %s; rename the rule's request model or that model's id",
			displayPath, strings.Join(result.Skipped, ", "))
	}

	if statErr == nil {
		backupPath, err := backupFile(targetPath)
		if err != nil {
			return fail("Failed to create backup: %v", err)
		}
		result.BackupPath = backupPath
		result.Updated = true
	} else {
		result.Created = true
	}

	output, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fail("Failed to marshal JSON: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fail("Failed to create directory: %v", err)
	}
	if err := writeFileAtomic(targetPath, append(output, '\n'), mode); err != nil {
		return fail("Failed to write %s: %v", displayPath, err)
	}

	result.Success = true
	switch {
	case result.Created:
		result.Message = fmt.Sprintf("Created %s", displayPath)
	case result.BackupPath != "":
		result.Message = fmt.Sprintf("Updated %s (backup: %s)", displayPath, result.BackupPath)
	default:
		result.Message = fmt.Sprintf("Updated %s", displayPath)
	}
	if len(result.Skipped) > 0 {
		result.Message += fmt.Sprintf("; skipped %s (id already used by another model)", strings.Join(result.Skipped, ", "))
	}
	return result
}

// trimUTF8BOM drops a leading UTF-8 byte-order mark. Hand-edited models.json
// files often carry one (Windows editors), and encoding/json rejects it.
func trimUTF8BOM(data []byte) []byte {
	return bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
}

// writeFileAtomic writes via a temp file in the same directory and renames, so
// WorkBuddy's hot reload never sees a half-written file.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".models-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return err
	}
	return nil
}

// ---- status ----

// WorkBuddyAppliedState is what the status check needs from one models.json.
type WorkBuddyAppliedState struct {
	Managed bool // has at least one entry pointing at this gateway's WorkBuddy scenario
	URL     string
	Models  []string
}

// CompareWorkBuddyConfig compares the applied managed entries with the models
// the current rules would produce.
func CompareWorkBuddyConfig(applied WorkBuddyAppliedState, models []string) (string, []ClientConfigDiff) {
	if !applied.Managed {
		return ClientConfigNotApplied, nil
	}
	var diffs []ClientConfigDiff
	if joinModels(applied.Models) != joinModels(models) {
		diffs = append(diffs, ClientConfigDiff{Key: "models", Applied: joinModels(applied.Models), Expected: joinModels(models)})
	}
	return stateFor(diffs), diffs
}

// CompareWorkBuddyTargets compares every installed edition's models.json with
// the models the current rules would produce: not applied while none of them
// has tingly entries, applied only when all of them match.
func CompareWorkBuddyTargets(targets []WorkBuddyTarget, models []string) (string, []ClientConfigDiff) {
	var diffs []ClientConfigDiff
	anyManaged := false
	for _, t := range targets {
		applied := readWorkBuddyAppliedStateAt(t.Path)
		anyManaged = anyManaged || applied.Managed
		state, d := CompareWorkBuddyConfig(applied, models)
		if state == ClientConfigNotApplied {
			d = []ClientConfigDiff{{Key: "models", Expected: joinModels(models)}}
		}
		if len(targets) > 1 {
			for i := range d {
				d[i].Key += " (" + t.DisplayPath + ")"
			}
		}
		diffs = append(diffs, d...)
	}
	if !anyManaged {
		return ClientConfigNotApplied, nil
	}
	return stateFor(diffs), diffs
}

// readWorkBuddyAppliedStateAt reads the tingly-managed entries from one
// models.json. A missing or unparsable file reads as not applied.
func readWorkBuddyAppliedStateAt(path string) WorkBuddyAppliedState {
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkBuddyAppliedState{}
	}
	var doc interface{}
	if json.Unmarshal(trimUTF8BOM(data), &doc) != nil {
		return WorkBuddyAppliedState{}
	}
	var models []interface{}
	switch d := doc.(type) {
	case []interface{}:
		models = d
	case map[string]interface{}:
		models, _ = d["models"].([]interface{})
	}
	state := WorkBuddyAppliedState{Models: []string{}}
	for _, e := range models {
		if !isWorkBuddyManaged(e) {
			continue
		}
		state.Managed = true
		m := e.(map[string]interface{})
		if state.URL == "" {
			state.URL, _ = m["url"].(string)
		}
		if id := entryID(e); id != "" {
			state.Models = append(state.Models, id)
		}
	}
	return state
}
