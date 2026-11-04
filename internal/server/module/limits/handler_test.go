package limits

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/internal/limits"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestStatusReportsLiveCounters(t *testing.T) {
	dir, err := os.MkdirTemp("", "limits-status-")
	require.NoError(t, err)
	defer os.RemoveAll(dir)
	cfg, err := config.NewConfig(config.WithConfigDir(dir))
	require.NoError(t, err)
	require.NoError(t, cfg.AddProvider(&typ.Provider{
		UUID: "acct", Name: "acct", Enabled: true, APIStyle: protocol.APIStyleOpenAI, APIBase: "https://x.example/v1",
		Limits: &ai.Limits{MaxConcurrent: 2},
	}))
	svc := &loadbalance.Service{Provider: "acct", Model: "m", Active: true, Limits: &ai.Limits{MaxRequests: 10, WindowSeconds: 60}}
	require.NoError(t, cfg.AddRequestConfig(typ.Rule{UUID: "r1", Scenario: typ.ScenarioOpenAI, RequestModel: "gpt", Services: []*loadbalance.Service{svc}, Active: true}))

	guard := routing.NewLimitsGuard(limits.NewRegistry(), cfg)
	p, ok := guard.Acquire("r1", svc, false)
	require.True(t, ok)
	defer p.Release()

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/limits/status", nil)
	NewHandler(cfg, guard, Deps{}).Status(c)
	require.Equal(t, http.StatusOK, w.Code)

	var resp LimitsStatusResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 2)
	byScope := map[string]LimitStatusEntry{}
	for _, e := range resp.Data {
		byScope[e.Scope] = e
	}
	require.Equal(t, 1, byScope["account"].Inflight)
	require.InDelta(t, 0.5, byScope["account"].Pressure, 1e-9)
	require.Equal(t, "ok", byScope["account"].State)
	require.Equal(t, int64(1), byScope["service"].Requests)
	require.Equal(t, "r1", byScope["service"].RuleUUID)

	// limits off -> empty list, not an error
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/limits/status", nil)
	NewHandler(cfg, nil, Deps{}).Status(c)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"data":[]`)
}

type fakeDirectory struct {
	teams map[string]string
	keys  []KeyInfo
}

func (d fakeDirectory) TeamName(id string) string { return d.teams[id] }
func (d fakeDirectory) KeysOfTeam(id string) []KeyInfo {
	var out []KeyInfo
	for _, k := range d.keys {
		if k.TeamID == id {
			out = append(out, k)
		}
	}
	return out
}
func (d fakeDirectory) KeyByUserID(id string) (KeyInfo, bool) {
	for _, k := range d.keys {
		if k.UserID == id {
			return k, true
		}
	}
	return KeyInfo{}, false
}

func policyEnv(t *testing.T) (*Handler, *routing.LimitsGuard, *gin.Engine, *[]ai.LimitPolicy) {
	t.Helper()
	dir, err := os.MkdirTemp("", "limits-policy-")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	cfg, err := config.NewConfig(config.WithConfigDir(dir))
	require.NoError(t, err)
	store, err := db.NewLimitPolicyStore(t.TempDir())
	require.NoError(t, err)
	guard := routing.NewLimitsGuard(limits.NewRegistry(), cfg)
	guard.SetPolicies(store)
	var changed []ai.LimitPolicy
	h := NewHandler(cfg, guard, Deps{
		Policies: store,
		Directory: fakeDirectory{
			teams: map[string]string{"t1": "Platform"},
			keys: []KeyInfo{
				{UserID: "u-alice", TeamID: "t1", DisplayName: "alice laptop", Enabled: true},
				{UserID: "u-bob", TeamID: "t1", DisplayName: "bob ci", Enabled: true},
			},
		},
		OnPolicyChanged: func(p ai.LimitPolicy) { changed = append(changed, p) },
	})
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/limits/status", h.Status)
	engine.GET("/limit-policies", h.ListPolicies)
	engine.PUT("/limit-policies/:kind/:ref", h.UpsertPolicy)
	engine.DELETE("/limit-policies/:kind/:ref", h.DeletePolicy)
	return h, guard, engine, &changed
}

func call(engine *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func TestPolicyAPI_ValidationAndRoundTrip(t *testing.T) {
	_, _, engine, changed := policyEnv(t)

	for _, bad := range []string{
		"/limit-policies/bogus/x",        // unknown kind
		"/limit-policies/team/nope",      // unknown team
		"/limit-policies/user/u-nobody",  // no such sharing key
		"/limit-policies/team_member/zz", // unknown team
	} {
		w := call(engine, "PUT", bad, `{"limits":{"max_concurrent":1}}`)
		require.Equal(t, http.StatusBadRequest, w.Code, bad+" "+w.Body.String())
	}
	require.Empty(t, *changed)

	w := call(engine, "PUT", "/limit-policies/user/u-alice",
		`{"limits":{"max_concurrent":2,"max_tokens":1000000,"window_seconds":18000,"soft_percent":80},"overflow":"reject"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var put LimitPolicyResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &put))
	require.True(t, put.Exists)
	require.Equal(t, 0, put.Data.Limits.SoftPercent, "soft steering does not apply to a person")
	require.Equal(t, ai.LimitOverflowReject, put.Data.Overflow)
	require.Len(t, *changed, 1, "saving a policy triggers its warm start")

	// enterprise identities have no key to validate against
	w = call(engine, "PUT", "/limit-policies/user/euser:emp-7", `{"limits":{"max_concurrent":1}}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	w = call(engine, "GET", "/limit-policies?kind=user", "")
	var list LimitPoliciesResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list.Data, 2)

	// an emptied editor deletes the policy
	w = call(engine, "PUT", "/limit-policies/user/u-alice", `{"limits":{}}`)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &put))
	require.False(t, put.Exists)
	w = call(engine, "GET", "/limit-policies?kind=user&ref=u-alice", "")
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Empty(t, list.Data)

	w = call(engine, "DELETE", "/limit-policies/user/euser:emp-7", "")
	require.Equal(t, http.StatusOK, w.Code)
}

func TestPolicyAPI_DisabledWithoutStore(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	h := NewHandler(nil, nil, Deps{})
	engine.PUT("/limit-policies/:kind/:ref", h.UpsertPolicy)
	w := call(engine, "PUT", "/limit-policies/team/t1", `{"limits":{"max_concurrent":1}}`)
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestStatus_ListsPersonTeamModelAndTemplateMembers(t *testing.T) {
	_, guard, engine, _ := policyEnv(t)
	require.Equal(t, http.StatusOK, call(engine, "PUT", "/limit-policies/team/t1", `{"limits":{"max_concurrent":4}}`).Code)
	require.Equal(t, http.StatusOK, call(engine, "PUT", "/limit-policies/team_member/t1", `{"limits":{"max_concurrent":2}}`).Code)
	require.Equal(t, http.StatusOK, call(engine, "PUT", "/limit-policies/user/u-bob", `{"limits":{"max_concurrent":1}}`).Code)
	require.Equal(t, http.StatusOK, call(engine, "PUT", "/limit-policies/model/claude-opus", `{"limits":{"max_concurrent":3,"soft_percent":70}}`).Code)

	// alice has one request in flight (the template applies to her)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/", nil)
	c.Set("user_id", "u-alice")
	c.Set("team_id", "t1")
	require.NoError(t, guard.AdmitActor(c, &routing.SelectionContext{}))

	w := call(engine, "GET", "/limits/status", "")
	var st LimitsStatusResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &st))
	find := func(key string) (LimitStatusEntry, bool) {
		for _, e := range st.Data {
			if e.Key == key {
				return e, true
			}
		}
		return LimitStatusEntry{}, false
	}

	team, ok := find("team:t1")
	require.True(t, ok)
	require.Equal(t, "Platform", team.Label)
	require.Equal(t, 1, team.Inflight, "alice's request counts against the team too")

	alice, ok := find("user:u-alice")
	require.True(t, ok)
	require.Equal(t, "alice laptop", alice.Label)
	require.Equal(t, "team_member", alice.InheritedFrom)
	require.Equal(t, 1, alice.Inflight)
	require.Equal(t, 2, alice.Limits.MaxConcurrent)
	require.InDelta(t, 0.5, alice.Pressure, 1e-9)

	bob, ok := find("user:u-bob")
	require.True(t, ok)
	require.Equal(t, "", bob.InheritedFrom, "explicit policy overrides the template")
	require.Equal(t, 1, bob.Limits.MaxConcurrent)

	model, ok := find("model:claude-opus")
	require.True(t, ok)
	require.Equal(t, 70, model.Limits.SoftPercent)

	tpl, ok := find("team_member:t1")
	require.True(t, ok)
	require.Equal(t, "template", tpl.InheritedFrom)
}
