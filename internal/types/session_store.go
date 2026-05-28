package types

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

const defaultSessionTTL = 30 * 24 * time.Hour

// MonicaSession 保存 Monica 多轮会话状态，供 Responses API 的 previous_response_id 续聊使用。
type MonicaSession struct {
	ConversationID  string    `json:"conversation_id"`
	Model           string    `json:"model"`
	BotUID          string    `json:"bot_uid"`
	Origin          string    `json:"origin"`
	OriginPageTitle string    `json:"origin_page_title"`
	Instructions    string    `json:"instructions"`
	LastQuestionItem Item     `json:"last_question_item"`
	LastReplyItem    Item     `json:"last_reply_item"`
	CreatedAt       time.Time `json:"created_at"`
}

// SessionStore 将 OpenAI response id 映射到 Monica 会话状态。
type SessionStore interface {
	Get(responseID string) (*MonicaSession, bool)
	Put(responseID string, session *MonicaSession)
}

// DefaultSessionStore 由 main 启动时通过 InitDiskSessionStore 初始化。
var DefaultSessionStore SessionStore

// IsStatefulRequest store 为 false 时为无状态（每轮独立会话，不读写缓存）。
// 未传 store 时默认有状态（与 OpenAI Responses API 默认 store=true 一致）。
func IsStatefulRequest(req CreateResponseRequest) bool {
	if req.Store == nil {
		return true
	}
	return *req.Store
}

// InitDiskSessionStore 使用本地磁盘目录持久化会话，TTL 默认 30 天。
func InitDiskSessionStore(dir string, ttl time.Duration) error {
	if dir == "" {
		return errors.New("session cache directory is required")
	}
	if ttl <= 0 {
		ttl = defaultSessionTTL
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolve cache dir: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}
	store := &diskSessionStore{dir: abs, ttl: ttl}
	if err := store.cleanupExpired(); err != nil {
		return fmt.Errorf("cleanup expired sessions: %w", err)
	}
	DefaultSessionStore = store
	return nil
}

type diskSessionStore struct {
	dir string
	ttl time.Duration
	mu  sync.Mutex
}

func (s *diskSessionStore) Get(responseID string) (*MonicaSession, bool) {
	path, err := s.sessionPath(responseID)
	if err != nil {
		return nil, false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}

	var session MonicaSession
	if err := json.Unmarshal(data, &session); err != nil {
		_ = os.Remove(path)
		return nil, false
	}
	if session.CreatedAt.IsZero() || time.Since(session.CreatedAt) > s.ttl {
		_ = os.Remove(path)
		return nil, false
	}
	return &session, true
}

func (s *diskSessionStore) Put(responseID string, session *MonicaSession) {
	if responseID == "" || session == nil {
		return
	}
	path, err := s.sessionPath(responseID)
	if err != nil {
		return
	}

	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if data, err := os.ReadFile(path); err == nil {
		var existing MonicaSession
		if json.Unmarshal(data, &existing) == nil && !existing.CreatedAt.IsZero() {
			session.CreatedAt = existing.CreatedAt
		}
	}

	data, err := json.Marshal(session)
	if err != nil {
		return
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}

func (s *diskSessionStore) sessionPath(responseID string) (string, error) {
	name := sanitizeSessionFilename(responseID)
	if name == "" {
		return "", errors.New("invalid response id")
	}
	return filepath.Join(s.dir, name+".json"), nil
}

func sanitizeSessionFilename(id string) string {
	var b strings.Builder
	b.Grow(len(id))
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

func (s *diskSessionStore) cleanupExpired() error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	now := time.Now()
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".json") {
			continue
		}
		path := filepath.Join(s.dir, ent.Name())
		info, err := ent.Info()
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) > s.ttl {
			_ = os.Remove(path)
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var session MonicaSession
		if err := json.Unmarshal(data, &session); err != nil {
			_ = os.Remove(path)
			continue
		}
		if session.CreatedAt.IsZero() || now.Sub(session.CreatedAt) > s.ttl {
			_ = os.Remove(path)
		}
	}
	return nil
}
