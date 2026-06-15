package server

import (
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// pluginNamespace is the UUIDv5 namespace used to derive a stable plugin id from
// its name, so a plugin that restarts re-registers under the same id and any
// durable rule that references it keeps pointing correctly.
var pluginNamespace = uuid.MustParse("3b1e0a2c-7c4e-5a9b-bf21-9f6d2c8e4a10")

const defaultPluginTTL = 30 * time.Second

// PluginRegistration is a live, ephemeral plugin instance.
type PluginRegistration struct {
	ID        string // deterministic from Name (UUIDv5)
	Name      string // plugin / provider name
	Endpoint  string // OpenAI base, e.g. http://127.0.0.1:8765/v1
	ModelID   string // advertised model id, e.g. plugin/my-rag
	Scenario  string // scenario the durable rule was bound under (optional)
	Token     string // token tb sends to the plugin (optional)
	LeaseID   string // rotates each register; required to heartbeat/deregister
	ExpiresAt time.Time
	LastSeen  time.Time
}

// PluginRegistry holds live plugin instances in memory. It is intentionally
// process-local (no shared store), matching tb's existing routing state stance.
// Instances auto-expire when their lease is not renewed; nothing is persisted.
type PluginRegistry struct {
	mu   sync.RWMutex
	byID map[string]*PluginRegistration
	ttl  time.Duration
}

// NewPluginRegistry creates an empty registry with the default lease TTL.
func NewPluginRegistry() *PluginRegistry {
	return &PluginRegistry{byID: map[string]*PluginRegistration{}, ttl: defaultPluginTTL}
}

// PluginID derives the stable id for a plugin name.
func PluginID(name string) string {
	return uuid.NewSHA1(pluginNamespace, []byte(name)).String()
}

// Register adds or refreshes a plugin instance and returns the live record.
// ttl <= 0 uses the registry default.
func (r *PluginRegistry) Register(name, endpoint, modelID, scenario, token string, ttl time.Duration) *PluginRegistration {
	if ttl <= 0 {
		ttl = r.ttl
	}
	if modelID == "" {
		modelID = "plugin/" + name
	}
	now := time.Now()
	reg := &PluginRegistration{
		ID:        PluginID(name),
		Name:      name,
		Endpoint:  endpoint,
		ModelID:   modelID,
		Scenario:  scenario,
		Token:     token,
		LeaseID:   uuid.NewString(),
		ExpiresAt: now.Add(ttl),
		LastSeen:  now,
	}
	r.mu.Lock()
	r.byID[reg.ID] = reg
	r.mu.Unlock()
	return reg
}

// Heartbeat extends the lease identified by leaseID. Returns false if no live
// registration matches (unknown or already expired).
func (r *PluginRegistry) Heartbeat(leaseID string, ttl time.Duration) bool {
	if ttl <= 0 {
		ttl = r.ttl
	}
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, reg := range r.byID {
		if reg.LeaseID == leaseID {
			if now.After(reg.ExpiresAt) {
				delete(r.byID, reg.ID)
				return false
			}
			reg.ExpiresAt = now.Add(ttl)
			reg.LastSeen = now
			return true
		}
	}
	return false
}

// Deregister removes the instance for leaseID. Returns true if one was removed.
func (r *PluginRegistry) Deregister(leaseID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, reg := range r.byID {
		if reg.LeaseID == leaseID {
			delete(r.byID, id)
			return true
		}
	}
	return false
}

// Resolve synthesizes a plugin-kind provider for a live instance by id. Expired
// instances are treated as absent (and reaped). Implements
// config.EphemeralProviderResolver.
func (r *PluginRegistry) Resolve(id string) (*typ.Provider, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	reg, ok := r.byID[id]
	if !ok {
		return nil, false
	}
	if time.Now().After(reg.ExpiresAt) {
		delete(r.byID, id)
		return nil, false
	}
	// A plugin is an ordinary OpenAI HTTP upstream plus the PluginDetail marker;
	// routing treats it like any other provider.
	return &typ.Provider{
		UUID:          reg.ID,
		Name:          reg.Name,
		APIBase:       reg.Endpoint,
		APIStyle:      "openai",
		Token:         reg.Token,
		NoKeyRequired: reg.Token == "",
		Enabled:       true,
		AuthType:      typ.AuthTypeAPIKey,
		Timeout:       constant.DefaultRequestTimeout,
		PluginDetail:  &typ.PluginDetail{ModelID: reg.ModelID},
	}, true
}

// List returns the currently-live registrations (expired ones are reaped).
func (r *PluginRegistry) List() []*PluginRegistration {
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*PluginRegistration, 0, len(r.byID))
	for id, reg := range r.byID {
		if now.After(reg.ExpiresAt) {
			delete(r.byID, id)
			continue
		}
		clone := *reg
		out = append(out, &clone)
	}
	return out
}
