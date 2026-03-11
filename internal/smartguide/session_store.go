package smartguide

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// SmartGuideSession 存储 @tb 的对话历史
type SmartGuideSession struct {
	ChatID         string           `json:"chat_id"`
	Platform       string           `json:"platform,omitempty"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
	CurrentProject string           `json:"current_project,omitempty"`
	Messages       []SessionMessage `json:"messages"`
}

// SessionMessage 表示会话中的一条消息
type SessionMessage struct {
	Role      string    `json:"role"` // "user" or "assistant"
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// SessionStore 管理 SmartGuide session 文件
type SessionStore struct {
	dataDir string
	mu      sync.RWMutex
}

// NewSessionStore 创建一个新的 SessionStore
func NewSessionStore(dataDir string) (*SessionStore, error) {
	if dataDir == "" {
		return nil, fmt.Errorf("dataDir is required")
	}

	// 确保目录存在
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create session directory: %w", err)
	}

	logrus.WithField("dataDir", dataDir).Info("Created SmartGuide session store")

	return &SessionStore{
		dataDir: dataDir,
	}, nil
}

// getSessionPath 获取 session 文件路径
func (s *SessionStore) getSessionPath(chatID string) string {
	// 使用 chatID 作为文件名，加上 -smartguide.json 后缀
	filename := chatID + "-smartguide.json"
	return filepath.Join(s.dataDir, filename)
}

// Load 加载指定 chatID 的 session
// 如果文件不存在，返回一个新的空 session
func (s *SessionStore) Load(chatID string) (*SmartGuideSession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	path := s.getSessionPath(chatID)

	// 检查文件是否存在
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// 文件不存在，返回新 session
		logrus.WithField("chatID", chatID).Debug("Session file not found, creating new session")
		return &SmartGuideSession{
			ChatID:    chatID,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Messages:  []SessionMessage{},
		}, nil
	}

	// 读取文件
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read session file: %w", err)
	}

	// 解析 JSON
	var sess SmartGuideSession
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, fmt.Errorf("failed to parse session file: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"chatID":    chatID,
		"msgCount":  len(sess.Messages),
		"createdAt": sess.CreatedAt,
	}).Debug("Loaded session from file")

	return &sess, nil
}

// Save 保存 session 到文件
func (s *SessionStore) Save(sess *SmartGuideSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sess == nil {
		return fmt.Errorf("session is nil")
	}

	// 更新时间戳
	sess.UpdatedAt = time.Now()

	// 序列化为 JSON
	data, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	// 写入文件
	path := s.getSessionPath(sess.ChatID)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write session file: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"chatID":   sess.ChatID,
		"msgCount": len(sess.Messages),
		"filePath": path,
	}).Debug("Saved session to file")

	return nil
}

// AddMessage 添加一条消息到 session 并立即保存
func (s *SessionStore) AddMessage(chatID string, msg SessionMessage) error {
	// 加载现有 session
	sess, err := s.Load(chatID)
	if err != nil {
		return fmt.Errorf("failed to load session: %w", err)
	}

	// 添加消息
	sess.Messages = append(sess.Messages, msg)

	// 保存
	if err := s.Save(sess); err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}

	return nil
}

// AddMessages 批量添加消息到 session 并保存
func (s *SessionStore) AddMessages(chatID string, messages []SessionMessage) error {
	if len(messages) == 0 {
		return nil
	}

	// 加载现有 session
	sess, err := s.Load(chatID)
	if err != nil {
		return fmt.Errorf("failed to load session: %w", err)
	}

	// 批量添加消息
	sess.Messages = append(sess.Messages, messages...)

	// 保存
	if err := s.Save(sess); err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}

	return nil
}

// UpdateCurrentProject 更新 session 的当前项目路径
func (s *SessionStore) UpdateCurrentProject(chatID string, projectPath string) error {
	sess, err := s.Load(chatID)
	if err != nil {
		return fmt.Errorf("failed to load session: %w", err)
	}

	sess.CurrentProject = projectPath

	return s.Save(sess)
}

// Delete 删除指定 chatID 的 session 文件
func (s *SessionStore) Delete(chatID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.getSessionPath(chatID)

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete session file: %w", err)
	}

	logrus.WithField("chatID", chatID).Info("Deleted session file")

	return nil
}

// List 列出所有 session 文件
func (s *SessionStore) List() ([]*SmartGuideSession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 读取目录中的所有文件
	entries, err := os.ReadDir(s.dataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read session directory: %w", err)
	}

	var sessions []*SmartGuideSession

	for _, entry := range entries {
		// 跳过目录
		if entry.IsDir() {
			continue
		}

		// 检查文件名是否以 -smartguide.json 结尾
		filename := entry.Name()
		if len(filename) < 17 || filename[len(filename)-17:] != "-smartguide.json" {
			continue
		}

		// 读取文件
		path := filepath.Join(s.dataDir, filename)
		data, err := os.ReadFile(path)
		if err != nil {
			logrus.WithError(err).WithField("file", filename).Warn("Failed to read session file")
			continue
		}

		// 解析
		var sess SmartGuideSession
		if err := json.Unmarshal(data, &sess); err != nil {
			logrus.WithError(err).WithField("file", filename).Warn("Failed to parse session file")
			continue
		}

		sessions = append(sessions, &sess)
	}

	return sessions, nil
}

// GetMessages 获取指定 chatID 的所有消息（不加载整个 session）
func (s *SessionStore) GetMessages(chatID string) ([]SessionMessage, error) {
	sess, err := s.Load(chatID)
	if err != nil {
		return nil, err
	}

	return sess.Messages, nil
}

// ClearMessages 清空指定 chatID 的所有消息
func (s *SessionStore) ClearMessages(chatID string) error {
	sess, err := s.Load(chatID)
	if err != nil {
		return fmt.Errorf("failed to load session: %w", err)
	}

	sess.Messages = []SessionMessage{}
	sess.UpdatedAt = time.Now()

	return s.Save(sess)
}
