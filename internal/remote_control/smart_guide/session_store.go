package smart_guide

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-agentscope/pkg/message"
	"github.com/tingly-dev/tingly-agentscope/pkg/module"
	agentscopeSession "github.com/tingly-dev/tingly-agentscope/pkg/session"
)

// SessionStore wraps agentscope session for SmartGuide message persistence
// Uses agentscope.Msg directly for message storage
type SessionStore struct {
	session agentscopeSession.Session
	mgr     *agentscopeSession.SessionManager
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
		mgr:     agentscopeSession.NewSessionManager(jsonSession),
	}, nil
}

// getSessionID returns the agentscope session ID for a chatID
func (s *SessionStore) getSessionID(chatID string) string {
	return chatID + "-smartguide"
}

// messageState wraps a slice of messages to implement StateModule
type messageState struct {
	*module.StateModuleBase
	messages []*message.Msg
}

// newMessageState creates a new message state
func newMessageState() *messageState {
	return &messageState{
		StateModuleBase: module.NewStateModuleBase(),
		messages:        nil,
	}
}

// StateDict returns the state as a dictionary
func (m *messageState) StateDict() map[string]any {
	// Convert messages to dict format
	msgDicts := make([]any, 0, len(m.messages))
	for _, msg := range m.messages {
		msgDicts = append(msgDicts, msg.ToDict())
	}
	return map[string]any{
		"messages": msgDicts,
	}
}

// LoadStateDict loads state from a dictionary
// Returns nil on error to avoid breaking the flow
func (m *messageState) LoadStateDict(ctx context.Context, state map[string]any) error {
	messagesRaw, ok := state["messages"]
	if !ok {
		// No messages in state, initialize empty
		m.messages = nil
		return nil
	}

	msgsAny, ok := messagesRaw.([]any)
	if !ok {
		// Invalid format, initialize empty
		logrus.WithField("type", fmt.Sprintf("%T", messagesRaw)).Warn("Session messages format invalid, returning empty")
		m.messages = nil
		return nil
	}

	m.messages = make([]*message.Msg, 0, len(msgsAny))
	for i, msgAny := range msgsAny {
		msgMap, ok := msgAny.(map[string]any)
		if !ok {
			logrus.WithField("index", i).Warn("Session message entry is not a map, skipping")
			continue
		}
		msg, err := message.FromDict(msgMap)
		if err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"index": i,
				"data":  fmt.Sprintf("%+v", msgMap),
			}).Warn("Failed to deserialize session message, skipping")
			continue
		}
		m.messages = append(m.messages, msg)
	}

	return nil
}

// getMessages returns all messages
func (m *messageState) getMessages() []*message.Msg {
	if m == nil {
		return nil
	}
	return m.messages
}

// setMessages sets all messages
func (m *messageState) setMessages(msgs []*message.Msg) {
	if m == nil {
		return
	}
	m.messages = msgs
}

// Load loads messages for a chatID, returns nil slice on error
func (s *SessionStore) Load(chatID string) ([]*message.Msg, error) {
	if s == nil {
		return nil, nil
	}

	ctx := context.Background()

	// Create a new message state for loading
	msgState := newMessageState()
	stateModules := map[string]module.StateModule{
		"messages": msgState,
	}

	if err := s.session.LoadSessionState(ctx, s.getSessionID(chatID), stateModules, true); err != nil {
		logrus.WithError(err).WithField("chatID", chatID).Debug("Failed to load session, returning empty")
		return nil, nil
	}

	messages := msgState.getMessages()

	logrus.WithFields(logrus.Fields{
		"chatID":   chatID,
		"msgCount": len(messages),
	}).Debug("Loaded messages from session")

	return messages, nil
}

// Save saves messages for a chatID
func (s *SessionStore) Save(chatID string, messages []*message.Msg) error {
	if s == nil || len(messages) == 0 {
		return nil
	}

	ctx := context.Background()

	// Create message state with messages
	msgState := newMessageState()
	msgState.setMessages(messages)

	stateModules := map[string]module.StateModule{
		"messages": msgState,
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
func (s *SessionStore) AddMessage(chatID string, msg *message.Msg) error {
	if s == nil || msg == nil {
		return nil
	}

	// Load existing messages
	messages, _ := s.Load(chatID)
	if messages == nil {
		messages = []*message.Msg{}
	}

	// Add new message
	messages = append(messages, msg)

	// Save
	return s.Save(chatID, messages)
}

// AddMessages adds multiple messages to the session
func (s *SessionStore) AddMessages(chatID string, newMessages []*message.Msg) error {
	if s == nil || len(newMessages) == 0 {
		return nil
	}

	// Load existing messages
	messages, _ := s.Load(chatID)
	if messages == nil {
		messages = []*message.Msg{}
	}

	// Append new messages
	messages = append(messages, newMessages...)

	// Save
	return s.Save(chatID, messages)
}

// GetMessages retrieves all messages for a chatID
func (s *SessionStore) GetMessages(chatID string) ([]*message.Msg, error) {
	if s == nil {
		return nil, nil
	}
	return s.Load(chatID)
}

// ClearMessages removes all messages for a chatID
func (s *SessionStore) ClearMessages(chatID string) error {
	if s == nil {
		return nil
	}

	ctx := context.Background()

	msgState := newMessageState()
	stateModules := map[string]module.StateModule{
		"messages": msgState,
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
		return nil, nil
	}

	ctx := context.Background()
	sessionIDs, err := s.session.ListSessions(ctx)
	if err != nil {
		return nil, nil
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
