package types

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const defaultSessionTTL = 30 * 24 * time.Hour

// MonicaSession 保存 Monica 多轮会话状态，供 Responses API 的 previous_response_id 续聊使用。
type MonicaSession struct {
	ConversationID   string    `json:"conversation_id"`
	Model            string    `json:"model"`
	BotUID           string    `json:"bot_uid"`
	Origin           string    `json:"origin"`
	OriginPageTitle  string    `json:"origin_page_title"`
	Instructions     string    `json:"instructions"`
	LastQuestionItem Item      `json:"last_question_item"`
	LastReplyItem    Item      `json:"last_reply_item"`
	CreatedAt        time.Time `json:"created_at"`
}

// SessionStore 将 OpenAI response id 映射到 Monica 会话状态。
type SessionStore interface {
	Get(responseID string) (*MonicaSession, bool)
	Put(responseID string, session *MonicaSession)
}

// DefaultSessionStore 由 main 启动时通过 InitSessionStore 初始化。
var DefaultSessionStore SessionStore

// IsStatefulRequest store 为 false 时为无状态（每轮独立会话，不读写缓存）。
func IsStatefulRequest(req CreateResponseRequest) bool {
	if req.Store == nil {
		return true
	}
	return *req.Store
}

// InitSessionStore 使用 SQLite 持久化会话（单文件 sessions.db）。
// cachePath 可为目录（在其下创建 sessions.db）或直接指向 .db 文件路径。
func InitSessionStore(cachePath string, ttl time.Duration) error {
	if cachePath == "" {
		return errors.New("session cache path is required")
	}
	if ttl <= 0 {
		ttl = defaultSessionTTL
	}

	dbPath, err := resolveSessionDBPath(cachePath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return fmt.Errorf("ping sqlite: %w", err)
	}

	store := &sqliteSessionStore{db: db, ttl: ttl, path: dbPath}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return fmt.Errorf("migrate sqlite: %w", err)
	}
	if err := store.cleanupExpired(); err != nil {
		_ = db.Close()
		return fmt.Errorf("cleanup expired sessions: %w", err)
	}

	DefaultSessionStore = store
	return nil
}

// ResolveSessionDBPath 解析缓存路径为 SQLite 数据库文件路径。
func ResolveSessionDBPath(cachePath string) (string, error) {
	return resolveSessionDBPath(cachePath)
}

func resolveSessionDBPath(cachePath string) (string, error) {
	cachePath = strings.TrimSpace(cachePath)
	if strings.HasSuffix(strings.ToLower(cachePath), ".db") {
		return filepath.Abs(cachePath)
	}
	abs, err := filepath.Abs(cachePath)
	if err != nil {
		return "", err
	}
	return filepath.Join(abs, "sessions.db"), nil
}

type sqliteSessionStore struct {
	db   *sql.DB
	ttl  time.Duration
	path string
}

func (s *sqliteSessionStore) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			response_id TEXT PRIMARY KEY NOT NULL,
			conversation_id TEXT NOT NULL,
			payload TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_sessions_created_at ON sessions(created_at);
	`)
	return err
}

func (s *sqliteSessionStore) Get(responseID string) (*MonicaSession, bool) {
	if responseID == "" {
		return nil, false
	}

	var payload string
	var createdAtUnix int64
	err := s.db.QueryRow(
		`SELECT payload, created_at FROM sessions WHERE response_id = ?`,
		responseID,
	).Scan(&payload, &createdAtUnix)
	if err != nil {
		return nil, false
	}

	createdAt := time.Unix(createdAtUnix, 0)
	if createdAt.IsZero() || time.Since(createdAt) > s.ttl {
		_, _ = s.db.Exec(`DELETE FROM sessions WHERE response_id = ?`, responseID)
		return nil, false
	}

	var session MonicaSession
	if err := json.Unmarshal([]byte(payload), &session); err != nil {
		_, _ = s.db.Exec(`DELETE FROM sessions WHERE response_id = ?`, responseID)
		return nil, false
	}
	session.CreatedAt = createdAt
	return &session, true
}

func (s *sqliteSessionStore) Put(responseID string, session *MonicaSession) {
	if responseID == "" || session == nil {
		return
	}

	now := time.Now()
	var existingCreated int64
	err := s.db.QueryRow(
		`SELECT created_at FROM sessions WHERE response_id = ?`,
		responseID,
	).Scan(&existingCreated)
	if err == nil {
		session.CreatedAt = time.Unix(existingCreated, 0)
	} else if session.CreatedAt.IsZero() {
		session.CreatedAt = now
	}

	payload, err := json.Marshal(session)
	if err != nil {
		return
	}

	_, err = s.db.Exec(`
		INSERT INTO sessions (response_id, conversation_id, payload, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(response_id) DO UPDATE SET
			conversation_id = excluded.conversation_id,
			payload = excluded.payload,
			updated_at = excluded.updated_at
	`,
		responseID,
		session.ConversationID,
		string(payload),
		session.CreatedAt.Unix(),
		now.Unix(),
	)
	if err != nil {
		return
	}
}

func (s *sqliteSessionStore) cleanupExpired() error {
	cutoff := time.Now().Add(-s.ttl).Unix()
	_, err := s.db.Exec(`DELETE FROM sessions WHERE created_at < ?`, cutoff)
	return err
}
