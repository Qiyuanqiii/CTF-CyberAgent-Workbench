package credential

import (
	"context"
	"errors"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

const (
	ProtocolVersion = "provider_credential.v1"
	// Windows Credential Manager caps a generic credential BLOB at 5*512 bytes.
	MaxSecretBytes = 5 * 512
)

var ErrUnavailable = errors.New("system credential storage is unavailable")

// Store owns Provider secrets inside Go. Public API projections may expose
// only Kind, Available, and configured status; Get is for Provider bootstrap.
type Store interface {
	Kind() string
	Available() bool
	Put(context.Context, string, string) error
	Delete(context.Context, string) error
	Get(context.Context, string) (string, bool, error)
	Configured(context.Context, string) (bool, error)
}

func NewSystemStore() Store {
	return newSystemStore()
}

func ValidName(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) ||
		len(value) > 64 {
		return false
	}
	for _, current := range value {
		if unicode.IsControl(current) || unicode.IsSpace(current) ||
			!(unicode.IsLetter(current) || unicode.IsDigit(current) || current == '-' || current == '_') {
			return false
		}
	}
	return true
}

func ValidSecret(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) ||
		len([]byte(value)) > MaxSecretBytes {
		return false
	}
	for _, current := range value {
		if unicode.IsControl(current) || unicode.IsSpace(current) {
			return false
		}
	}
	return true
}

type MemoryStore struct {
	mu     sync.RWMutex
	values map[string]string
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{values: make(map[string]string)}
}

func (*MemoryStore) Kind() string    { return "memory_test_only" }
func (*MemoryStore) Available() bool { return true }

func (s *MemoryStore) Put(ctx context.Context, name string, secret string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !ValidName(name) || !ValidSecret(secret) {
		return errors.New("credential name or secret is invalid")
	}
	s.mu.Lock()
	s.values[name] = secret
	s.mu.Unlock()
	return nil
}

func (s *MemoryStore) Delete(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !ValidName(name) {
		return errors.New("credential name is invalid")
	}
	s.mu.Lock()
	delete(s.values, name)
	s.mu.Unlock()
	return nil
}

func (s *MemoryStore) Get(ctx context.Context, name string) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	if !ValidName(name) {
		return "", false, errors.New("credential name is invalid")
	}
	s.mu.RLock()
	value, found := s.values[name]
	s.mu.RUnlock()
	return value, found, nil
}

func (s *MemoryStore) Configured(ctx context.Context, name string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if !ValidName(name) {
		return false, errors.New("credential name is invalid")
	}
	s.mu.RLock()
	_, found := s.values[name]
	s.mu.RUnlock()
	return found, nil
}
