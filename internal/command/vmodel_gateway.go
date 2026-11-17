package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/tingly-dev/tingly-box/internal/app"
	"github.com/tingly-dev/tingly-box/internal/appconfig"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/lock"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Registration uses the running server's APIs, never its SQLite database.
type vmodelGateway struct {
	baseURL string
	token   string
	client  *http.Client
}

func runningVModelGateway(am *app.AppManager) (*vmodelGateway, error) {
	if !lock.NewFileLock(am.ConfigDir()).IsLocked() {
		return nil, fmt.Errorf("tb is not running; run 'tb start' first, or use 'tb vmodel --standalone'")
	}
	return &vmodelGateway{
		baseURL: fmt.Sprintf("http://127.0.0.1:%d", am.GetRuntimeServerPort()),
		token:   appconfig.UserTokenFromFile(am.ConfigDir()),
		client:  &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (g *vmodelGateway) api(ctx context.Context, method, path string, body, dst any) error {
	var buffer bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buffer).Encode(body); err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, g.baseURL+path, &buffer)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.token)
	resp, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("tb %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	if method == http.MethodDelete && resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("tb %s %s returned HTTP %d", method, path, resp.StatusCode)
	}
	var reply struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&reply); err != nil {
		return err
	}
	if !reply.Success {
		return fmt.Errorf("tb %s %s failed", method, path)
	}
	if dst != nil {
		return json.Unmarshal(reply.Data, dst)
	}
	return nil
}

func (g *vmodelGateway) attach(ctx context.Context, ready *vmodelReady) (func() error, error) {
	// Model IDs remain the request model names. Never replace a user's rule.
	var existing []typ.Rule
	if err := g.api(ctx, "GET", "/api/v1/rules?scenario=vmodel", nil, &existing); err != nil {
		return nil, err
	}
	for _, r := range existing {
		for _, id := range ready.Models {
			if r.RequestModel == id {
				return nil, fmt.Errorf("vmodel rule for %q already exists; choose a different model id or use --standalone", id)
			}
		}
	}
	var rules, providers []string
	cleanup := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		var failures []error
		for i := len(rules) - 1; i >= 0; i-- {
			if err := g.api(ctx, "DELETE", "/api/v1/rule/"+url.PathEscape(rules[i]), nil, nil); err != nil {
				failures = append(failures, err)
			}
		}
		for i := len(providers) - 1; i >= 0; i-- {
			if err := g.api(ctx, "DELETE", "/api/v2/providers/"+url.PathEscape(providers[i]), nil, nil); err != nil {
				failures = append(failures, err)
			}
		}
		return errors.Join(failures...)
	}
	fail := func(err error) (func() error, error) { return nil, errors.Join(err, cleanup()) }
	selected := make(map[string]string)
	instance := uuid.NewString()
	for _, source := range []struct{ style, base string }{{"openai", ready.OpenAIBaseURL}, {"anthropic", ready.AnthropicBaseURL + "/v1"}} {
		// Read the actual per-protocol list instead of assuming every builtin
		// speaks both protocols. Shared models prefer the OpenAI provider.
		req, err := http.NewRequestWithContext(ctx, "GET", source.base+"/models", nil)
		if err != nil {
			return fail(err)
		}
		resp, err := g.client.Do(req)
		if err != nil {
			return fail(err)
		}
		var list struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		err = json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&list)
		resp.Body.Close()
		if err != nil {
			return fail(err)
		}
		if resp.StatusCode != http.StatusOK {
			return fail(fmt.Errorf("vmodel model list returned HTTP %d", resp.StatusCode))
		}
		var ids []string
		for _, model := range list.Data {
			if _, ok := selected[model.ID]; !ok {
				ids = append(ids, model.ID)
			}
		}
		if len(ids) == 0 {
			continue
		}
		base := ready.OpenAIBaseURL
		if source.style == "anthropic" {
			base = ready.AnthropicBaseURL
		}
		var provider struct {
			UUID string `json:"uuid"`
		}
		if err := g.api(ctx, "POST", "/api/v2/providers", map[string]any{
			"name": "VModel " + source.style + " " + instance, "api_base": base,
			"api_style": source.style, "auth_type": "api_key", "no_key_required": true, "enabled": true,
		}, &provider); err != nil {
			return fail(err)
		}
		if provider.UUID == "" {
			return fail(fmt.Errorf("tb returned an empty provider UUID"))
		}
		providers = append(providers, provider.UUID)
		for _, id := range ids {
			selected[id] = provider.UUID
		}
	}
	ids := make([]string, 0, len(selected))
	for id := range selected {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		var rule struct {
			UUID string `json:"uuid"`
		}
		if err := g.api(ctx, "POST", "/api/v1/rule", typ.Rule{
			Scenario: typ.ScenarioVModel, RequestModel: id, Active: true,
			Description: "Temporary route managed by tb vmodel (" + instance + ")",
			Services:    []*loadbalance.Service{{Provider: selected[id], Model: id, Active: true, Weight: 1}},
		}, &rule); err != nil {
			return fail(err)
		}
		if rule.UUID == "" {
			return fail(fmt.Errorf("tb returned an empty rule UUID"))
		}
		rules = append(rules, rule.UUID)
	}
	ready.GatewayBaseURL = g.baseURL + "/tingly/vmodel"
	ready.Scenario = "vmodel"
	return cleanup, nil
}
