package smart_guide

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-agentscope/pkg/module"
	agentscopeSession "github.com/tingly-dev/tingly-agentscope/pkg/session"
)

// SessionMessage represents a message in the conversation
type SessionMessage struct {
	Role      string    `json:"role"` // "user" or "assistant"
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// messageState implements module.StateModule for storing message history
type messageState struct {
	*module.StateModuleBase
}

// newMessageState creates a new message state
func newMessageState() *messageState {
	return &messageState{
		StateModuleBase: module.NewStateModuleBase(),
	}
}

// getMessages returns all stored messages as SessionMessage slice
func (m *messageState) getMessages() []SessionMessage {
	if m == nil {
		return []SessionMessage{}
	}

	// Get messages from state
	val, ok := m.Get("messages")
	if !ok {
		return []SessionMessage{}
	}

	// Convert to []SessionMessage
	if msgs, ok := val.([]SessionMessage); ok {
		return msgs
	}

	return []SessionMessage{}
}

// setMessages stores messages
func (m *messageState) setMessages(msgs []SessionMessage) {
	if m == nil {
		return
	}
	m.Set("messages", msgs)
}

// SessionStore wraps agentscope session for SmartGuide message persistence
type SessionStore struct {
	session agentscopeSession.Session
	state   *messageState
}

// NewSessionStore creates a new session store using agentscope
func NewSessionStore(dataDir string) (*SessionStore, error) {
	if dataDir == "" {
		return nil, nil
	}

	jsonSession := agentscopeSession.NewJSONSession(dataDir)

	logrus.WithField("dataDir", dataDir).Info("Created SmartGuide session store (agentscope)")

	return &SessionStore{
		session: jsonSession,
		state:   newMessageState(),
	}, nil
}

// getSessionID returns the agentscope session ID for a chatID
func (s *SessionStore) getSessionID(chatID string) string {
	return chatID + "-smartguide"
}

// Load loads messages for a chatID, returns empty slice on error
func (s *SessionStore) Load(chatID string) ([]SessionMessage, error) {
	if s == nil {
		return []SessionMessage{}, nil
	}

	ctx := context.Background()

	// Load session state (allow not exist)
	stateModules := map[string]module.StateModule{
		"messages": s.state,
	}

	if err := s.session.LoadSessionState(ctx, s.getSessionID(chatID), stateModules, true); err != nil {
		logrus.WithError(err).WithField("chatID", chatID).Debug("Failed to load session, returning empty")
		return []SessionMessage{}, nil
	}

	messages := s.state.getMessages()

	logrus.WithFields(logrus.Fields{
		"chatID":   chatID,
		"msgCount": len(messages),
	}).Debug("Loaded messages from session")

	return messages, nil
}

// Save saves messages for a chatID
func (s *SessionStore) Save(chatID string, messages []SessionMessage) error {
	if s == nil || len(messages) == 0 {
		return nil
	}

	ctx := context.Background()

	// Store messages in state
	s.state.setMessages(messages)

	// Save session
	stateModules := map[string]module.StateModule{
		"messages": s.state,
	}

	if err := s.session.SaveSessionState(ctx, s.getSessionID(chatID), stateModules); err != nil {
		return err
	}

	logrus.WithFields(logrus.Fields{
		"chatID":   chatID,
		"msgCount": len(messages),
	}).Debug("Saved messages to session")

	return nil
}

// AddMessage adds a single message to the session
func (s *SessionStore) AddMessage(chatID string, msg SessionMessage) error {
	if s == nil {
		return nil
	}

	// Load existing messages
	messages, _ := s.Load(chatID)

	// Add new message
	messages = append(messages, msg)

	// Save
	return s.Save(chatID, messages)
}

// AddMessages adds multiple messages to the session
func (s *SessionStore) AddMessages(chatID string, newMessages []SessionMessage) error {
	if s == nil || len(newMessages) == 0 {
		return nil
	}

	// Load existing messages
	messages, _ := s.Load(chatID)

	// Append new messages
	messages = append(messages, newMessages...)

	// Save
	return s.Save(chatID, messages)
}

// GetMessages retrieves all messages for a chatID
func (s *SessionStore) GetMessages(chatID string) ([]SessionMessage, error) {
	if s == nil {
		return []SessionMessage{}, nil
	}
	return s.Load(chatID)
}

// ClearMessages removes all messages for a chatID
func (s *SessionStore) ClearMessages(chatID string) error {
	if s == nil {
		return nil
	}

	ctx := context.Background()
	s.state.Clear()

	stateModules := map[string]module.StateModule{
		"messages": s.state,
	}

	if err := s.session.SaveSessionState(ctx, s.getSessionID(chatID), stateModules); err != nil {
		return err
	}

	logrus.WithField("chatID", chatID).Debug("Cleared session messages")

	return nil
}

// Delete removes the session for a chatID
func (s *SessionStore) Delete(chatID string) error {
	if s == nil {
		return nil
	}

	ctx := context.Background()
	if err := s.session.DeleteSession(ctx, s.getSessionID(chatID)); err != nil {
		return err
	}

	logrus.WithField("chatID", chatID).Debug("Deleted session")

	return nil
}

// List returns all chat IDs with sessions
func (s *SessionStore) List() ([]string, error) {
	if s == nil {
		return []string{}, nil
	}

	ctx := context.Background()
	sessionIDs, err := s.session.ListSessions(ctx)
	if err != nil {
		return []string{}, nil
	}

	// Filter only smartguide sessions
	var result []string
	for _, sessionID := range sessionIDs {
		if len(sessionID) > 11 && sessionID[len(sessionID)-11:] == "-smartguide" {
			chatID := sessionID[:len(sessionID)-11]
			result = append(result, chatID)
		}
	}

	return result, nil
}

// UpdateCurrentProject is a no-op (project is now managed by ChatStore)
func (s *SessionStore) UpdateCurrentProject(chatID, projectPath string) error {
	return nil
}
