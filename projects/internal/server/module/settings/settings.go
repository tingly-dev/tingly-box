// Package settings serves the model connection: where chats are sent, with
// which key and which default model. It also owns the connection test, which
// sends a real chat request through the same client the chat module uses.
package settings

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/projects/internal/db"
	"github.com/tingly-dev/tingly-box/projects/internal/llm"
	"github.com/tingly-dev/tingly-box/projects/internal/project"
)

const settingsKey = "connection"

// Default connection: a local Tingly Box gateway on its default port, using
// its Anthropic-compatible surface.
const (
	DefaultAPIStyle = llm.StyleAnthropic
	DefaultBaseURL  = "http://localhost:12580/tingly/anthropic"
)

// Connection is the persisted settings section.
type Connection struct {
	APIStyle     string `json:"api_style"`
	BaseURL      string `json:"base_url"`
	APIKey       string `json:"api_key"`
	DefaultModel string `json:"default_model"`
}

// LLM returns the llm-level connection.
func (c Connection) LLM() llm.Connection {
	return llm.Connection{APIStyle: c.APIStyle, BaseURL: c.BaseURL, APIKey: c.APIKey}
}

// Service reads and writes the connection and caches the model list.
type Service struct {
	store *db.Store

	mu          sync.Mutex
	models      []llm.Model
	modelsErr   error
	modelsAt    time.Time
	modelsForID string
}

// NewService builds the settings service.
func NewService(store *db.Store) *Service {
	return &Service{store: store}
}

// Connection returns the saved connection, or the defaults if none was saved.
func (s *Service) Connection() (Connection, error) {
	conn := Connection{APIStyle: DefaultAPIStyle, BaseURL: DefaultBaseURL}
	if _, err := s.store.GetSetting(settingsKey, &conn); err != nil {
		return conn, err
	}
	if conn.APIStyle == "" {
		conn.APIStyle = DefaultAPIStyle
	}
	return conn, nil
}

// Save persists conn and drops the cached model list.
func (s *Service) Save(conn Connection) error {
	if err := s.store.PutSetting(settingsKey, conn); err != nil {
		return err
	}
	s.mu.Lock()
	s.modelsAt = time.Time{}
	s.mu.Unlock()
	return nil
}

// Client builds an llm client for the saved connection.
func (s *Service) Client() (llm.Client, Connection, error) {
	conn, err := s.Connection()
	if err != nil {
		return nil, conn, err
	}
	c, err := llm.New(conn.LLM())
	return c, conn, err
}

const modelsTTL = time.Minute

// ModelsFor lists models for conn, cached for a minute per connection.
func (s *Service) ModelsFor(ctx context.Context, conn Connection) ([]llm.Model, error) {
	id := conn.APIStyle + "|" + conn.BaseURL + "|" + conn.APIKey
	s.mu.Lock()
	if s.modelsForID == id && time.Since(s.modelsAt) < modelsTTL {
		models, err := s.models, s.modelsErr
		s.mu.Unlock()
		return models, err
	}
	s.mu.Unlock()

	client, err := llm.New(conn.LLM())
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	models, err := client.ListModels(ctx)

	s.mu.Lock()
	s.models, s.modelsErr, s.modelsAt, s.modelsForID = models, err, time.Now(), id
	s.mu.Unlock()
	return models, err
}

// Models lists models for the saved connection. With ThreadEnv and
// CoordinatorEnv it makes the settings service the standalone app's
// project.Host.
func (s *Service) Models(ctx context.Context) ([]project.Model, error) {
	conn, err := s.Connection()
	if err != nil {
		return nil, err
	}
	models, err := s.ModelsFor(ctx, conn)
	if err != nil {
		return nil, err
	}
	out := make([]project.Model, 0, len(models))
	for _, m := range models {
		out = append(out, project.Model{ID: m.ID, DisplayName: m.DisplayName})
	}
	return out, nil
}

// ResolveModel picks the model for a request: the explicit one, else the
// configured default, else the first model the endpoint lists.
func (s *Service) ResolveModel(ctx context.Context, conn Connection, explicit string) (string, error) {
	if m := strings.TrimSpace(explicit); m != "" {
		return m, nil
	}
	if conn.DefaultModel != "" {
		return conn.DefaultModel, nil
	}
	models, err := s.ModelsFor(ctx, conn)
	if err != nil {
		return "", fmt.Errorf("no default model is set and listing models failed: %w", err)
	}
	if len(models) == 0 {
		return "", fmt.Errorf("no default model is set and the endpoint lists no models")
	}
	return models[0].ID, nil
}

// ThreadEnv points Claude Code threads at the saved connection when it
// speaks the Anthropic API (the default: a local Tingly Box), so threads go
// through the same gateway and model routing. Otherwise threads use the
// CLI's own login and default model.
func (s *Service) ThreadEnv(ctx context.Context) ([]string, string) {
	conn, err := s.Connection()
	if err != nil || conn.APIStyle != llm.StyleAnthropic || conn.BaseURL == "" {
		return nil, ""
	}
	env := []string{"ANTHROPIC_BASE_URL=" + strings.TrimRight(conn.BaseURL, "/")}
	if conn.APIKey != "" {
		env = append(env, "ANTHROPIC_AUTH_TOKEN="+conn.APIKey, "ANTHROPIC_API_KEY=")
	}
	model, _ := s.ResolveModel(ctx, conn, "")
	return env, model
}

// CoordinatorEnv requires a Claude-compatible endpoint rather than silently
// switching a configured OpenAI connection to the CLI's unrelated login.
func (s *Service) CoordinatorEnv(ctx context.Context) ([]string, string, error) {
	conn, err := s.Connection()
	if err != nil {
		return nil, "", err
	}
	if conn.APIStyle != llm.StyleAnthropic || conn.BaseURL == "" {
		return nil, "", fmt.Errorf("the coordinator requires an Anthropic-compatible connection or a Claude Code profile")
	}
	env, model := s.ThreadEnv(ctx)
	return env, model, nil
}
