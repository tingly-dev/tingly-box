package guardrails

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var ErrProtectedCredentialNotFound = errors.New("protected credential not found")

type ProtectedCredentialStore struct {
	path string
	mu   sync.Mutex
}

func NewProtectedCredentialStore(path string) *ProtectedCredentialStore {
	return &ProtectedCredentialStore{path: path}
}

func (s *ProtectedCredentialStore) List() ([]ProtectedCredential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

func (s *ProtectedCredentialStore) Create(credential ProtectedCredential) (ProtectedCredential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	credentials, err := s.load()
	if err != nil {
		return ProtectedCredential{}, err
	}
	for _, existing := range credentials {
		if existing.ID == credential.ID {
			return ProtectedCredential{}, fmt.Errorf("protected credential already exists")
		}
		if existing.AliasToken == credential.AliasToken {
			return ProtectedCredential{}, fmt.Errorf("protected credential alias token already exists")
		}
	}
	credentials = append(credentials, credential)
	if err := s.save(credentials); err != nil {
		return ProtectedCredential{}, err
	}
	return credential, nil
}

func (s *ProtectedCredentialStore) Update(id string, update func(*ProtectedCredential) error) (ProtectedCredential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	credentials, err := s.load()
	if err != nil {
		return ProtectedCredential{}, err
	}
	for i := range credentials {
		if credentials[i].ID != id {
			continue
		}
		if err := update(&credentials[i]); err != nil {
			return ProtectedCredential{}, err
		}
		credentials[i].UpdatedAt = time.Now().UTC()
		if err := s.save(credentials); err != nil {
			return ProtectedCredential{}, err
		}
		return credentials[i], nil
	}
	return ProtectedCredential{}, ErrProtectedCredentialNotFound
}

func (s *ProtectedCredentialStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	credentials, err := s.load()
	if err != nil {
		return err
	}
	next := make([]ProtectedCredential, 0, len(credentials))
	found := false
	for _, credential := range credentials {
		if credential.ID == id {
			found = true
			continue
		}
		next = append(next, credential)
	}
	if !found {
		return ErrProtectedCredentialNotFound
	}
	return s.save(next)
}

func (s *ProtectedCredentialStore) Resolve(ids []string) ([]ProtectedCredential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	credentials, err := s.load()
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	byID := make(map[string]ProtectedCredential, len(credentials))
	for _, credential := range credentials {
		byID[credential.ID] = credential
	}
	resolved := make([]ProtectedCredential, 0, len(ids))
	for _, id := range ids {
		credential, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("protected credential %q not found", id)
		}
		resolved = append(resolved, credential)
	}
	return resolved, nil
}

func (s *ProtectedCredentialStore) load() ([]ProtectedCredential, error) {
	if s.path == "" {
		return nil, fmt.Errorf("protected credential store path is empty")
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []ProtectedCredential{}, nil
		}
		return nil, err
	}
	var credentials []ProtectedCredential
	if err := json.Unmarshal(data, &credentials); err != nil {
		return nil, fmt.Errorf("decode protected credentials: %w", err)
	}
	return credentials, nil
}

func (s *ProtectedCredentialStore) save(credentials []ProtectedCredential) error {
	if s.path == "" {
		return fmt.Errorf("protected credential store path is empty")
	}
	data, err := json.MarshalIndent(credentials, "", "  ")
	if err != nil {
		return fmt.Errorf("encode protected credentials: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func UpdateProtectedCredential(existing *ProtectedCredential, name, credentialType, secret, description string, tags []string, enabled bool) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("credential name is required")
	}
	if !IsSupportedProtectedCredentialType(credentialType) {
		return fmt.Errorf("unsupported credential type %q", credentialType)
	}
	existing.Name = strings.TrimSpace(name)
	existing.Type = strings.TrimSpace(credentialType)
	if secret != "" {
		existing.Secret = secret
	}
	existing.Description = strings.TrimSpace(description)
	existing.Tags = normalizeStringSlice(tags)
	existing.Enabled = enabled
	return nil
}
