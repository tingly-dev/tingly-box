package spec

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/vmodel/script"
)

// scriptControlPrefix is where a driver talks to the scripts of a runtime;
// it cannot collide with a protocol route.
const scriptControlPrefix = "/_vmodel/scripts"

// Scripts are the runtime's script engines by id.
func (r *Runtime) Scripts() map[string]*script.Engine {
	out := make(map[string]*script.Engine, len(r.scripts))
	for id, e := range r.scripts {
		out[id] = e
	}
	return out
}

// ScriptIDs are the ids of the runtime's scripts, sorted.
func (r *Runtime) ScriptIDs() []string {
	ids := make([]string, 0, len(r.scripts))
	for id := range r.scripts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// scriptFor is the script that answers model id, directly or as fallback.
func (r *Runtime) scriptFor(id string) *script.Engine {
	if e, ok := r.scripts[id]; ok {
		return e
	}
	if _, ok := r.definitions[id]; !ok && r.fallback != "" {
		return r.scripts[r.fallback]
	}
	return nil
}

// serveScriptCountTokens answers count_tokens for a script's model; false
// leaves the request to the usual handlers.
func (r *Runtime) serveScriptCountTokens(w http.ResponseWriter, req *http.Request) bool {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		writeRequestError(w, req, 400, "read request body")
		return true
	}
	req.Body = io.NopCloser(strings.NewReader(string(body)))
	var in struct {
		Model string `json:"model"`
	}
	_ = json.Unmarshal(body, &in)
	e := r.scriptFor(in.Model)
	if e == nil {
		return false
	}
	e.ServeHTTP(w, req)
	return true
}

// serveScriptControl is the driver's side of a script run:
//
//	GET  /_vmodel/scripts                         ids and reports
//	GET  /_vmodel/scripts/{id}/report             the report so far
//	GET  /_vmodel/scripts/{id}/trace              every request so far
//	GET  /_vmodel/scripts/{id}/served?step=t.s    wait until a step was answered (timeout=30s; 408)
//	GET  /_vmodel/scripts/{id}/reached?step=t.s   wait until a request claimed a step (its reply may be held)
//	POST /_vmodel/scripts/{id}/signals/{name}     raise a signal
//	POST /_vmodel/scripts/{id}/finish             end the run, return the final report
func (r *Runtime) serveScriptControl(w http.ResponseWriter, req *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(req.URL.Path, scriptControlPrefix), "/")
	if rest == "" {
		if req.Method != http.MethodGet {
			writeJSON(w, 405, map[string]string{"error": "use GET"})
			return
		}
		out := []map[string]any{}
		for _, id := range r.ScriptIDs() {
			out = append(out, map[string]any{"id": id, "report": r.scripts[id].Report()})
		}
		writeJSON(w, 200, map[string]any{"scripts": out})
		return
	}
	parts := strings.Split(rest, "/")
	e, ok := r.scripts[parts[0]]
	if !ok {
		writeJSON(w, 404, map[string]string{"error": "no script " + parts[0]})
		return
	}
	action := strings.Join(parts[1:], "/")
	switch {
	case req.Method == http.MethodGet && action == "report":
		writeJSON(w, 200, e.Report())
	case req.Method == http.MethodGet && action == "trace":
		writeJSON(w, 200, map[string]any{"entries": e.Trace()})
	case req.Method == http.MethodGet && (action == "served" || action == "reached"):
		timeout := 30 * time.Second
		if v := req.URL.Query().Get("timeout"); v != "" {
			d, err := time.ParseDuration(v)
			if err != nil {
				writeJSON(w, 400, map[string]string{"error": "timeout: " + err.Error()})
				return
			}
			timeout = d
		}
		ctx, cancel := context.WithTimeout(req.Context(), timeout)
		defer cancel()
		wait := e.WaitServed
		if action == "reached" {
			wait = e.WaitReached
		}
		if err := wait(ctx, req.URL.Query().Get("step")); err != nil {
			status := 408
			if ctx.Err() == nil {
				status = 400
			}
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]bool{"served": true})
	case req.Method == http.MethodPost && len(parts) == 3 && parts[1] == "signals" && parts[2] != "":
		e.Signal(parts[2])
		w.WriteHeader(http.StatusNoContent)
	case req.Method == http.MethodPost && action == "finish":
		writeJSON(w, 200, e.Finish())
	default:
		writeJSON(w, 404, map[string]string{"error": "no script action " + req.Method + " " + action})
	}
}
