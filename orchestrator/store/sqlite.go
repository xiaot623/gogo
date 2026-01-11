package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/xiaot623/gogo/orchestrator/domain"
	_ "github.com/mattn/go-sqlite3"
)

// SQLiteStore implements Store using SQLite.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore creates a new SQLite store.
func NewSQLiteStore(dsn string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	store := &SQLiteStore{db: db}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return store, nil
}

// migrate runs database migrations.
func (s *SQLiteStore) migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			session_id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			metadata TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			message_id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			run_id TEXT,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			metadata TEXT,
			FOREIGN KEY (session_id) REFERENCES sessions(session_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id, created_at)`,
		`CREATE TABLE IF NOT EXISTS runs (
			run_id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			root_agent_id TEXT NOT NULL,
			parent_run_id TEXT,
			status TEXT NOT NULL,
			started_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			ended_at DATETIME,
			error TEXT,
			FOREIGN KEY (session_id) REFERENCES sessions(session_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_runs_session ON runs(session_id, started_at)`,
		`CREATE TABLE IF NOT EXISTS events (
			event_id TEXT PRIMARY KEY,
			run_id TEXT NOT NULL,
			ts INTEGER NOT NULL,
			type TEXT NOT NULL,
			payload TEXT,
			FOREIGN KEY (run_id) REFERENCES runs(run_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_events_run ON events(run_id, ts)`,
		`CREATE TABLE IF NOT EXISTS agents (
			agent_id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			endpoint TEXT NOT NULL,
			capabilities TEXT,
			status TEXT NOT NULL DEFAULT 'healthy',
			last_heartbeat DATETIME,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	for _, m := range migrations {
		if _, err := s.db.Exec(m); err != nil {
			return fmt.Errorf("migration failed: %w\n%s", err, m)
		}
	}

	return nil
}

// Close closes the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// CreateSession creates a new session.
func (s *SQLiteStore) CreateSession(ctx context.Context, session *domain.Session) error {
	metadata, _ := json.Marshal(session.Metadata)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (session_id, user_id, created_at, metadata) VALUES (?, ?, ?, ?)`,
		session.SessionID, session.UserID, session.CreatedAt, string(metadata))
	return err
}

// GetSession retrieves a session by ID.
func (s *SQLiteStore) GetSession(ctx context.Context, sessionID string) (*domain.Session, error) {
	var session domain.Session
	var metadata sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT session_id, user_id, created_at, metadata FROM sessions WHERE session_id = ?`,
		sessionID).Scan(&session.SessionID, &session.UserID, &session.CreatedAt, &metadata)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if metadata.Valid {
		session.Metadata = json.RawMessage(metadata.String)
	}
	return &session, nil
}

// GetOrCreateSession gets an existing session or creates a new one.
func (s *SQLiteStore) GetOrCreateSession(ctx context.Context, sessionID, userID string) (*domain.Session, error) {
	session, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session != nil {
		return session, nil
	}

	// Create new session
	session = &domain.Session{
		SessionID: sessionID,
		UserID:    userID,
		CreatedAt: time.Now(),
	}
	if err := s.CreateSession(ctx, session); err != nil {
		return nil, err
	}
	return session, nil
}

// CreateMessage creates a new message.
func (s *SQLiteStore) CreateMessage(ctx context.Context, message *domain.Message) error {
	metadata, _ := json.Marshal(message.Metadata)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO messages (message_id, session_id, run_id, role, content, created_at, metadata) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		message.MessageID, message.SessionID, message.RunID, message.Role, message.Content, message.CreatedAt, string(metadata))
	return err
}

// GetMessages retrieves messages for a session.
func (s *SQLiteStore) GetMessages(ctx context.Context, sessionID string, limit int, before string) ([]domain.Message, error) {
	query := `SELECT message_id, session_id, run_id, role, content, created_at, metadata FROM messages WHERE session_id = ?`
	args := []interface{}{sessionID}

	if before != "" {
		query += ` AND message_id < ?`
		args = append(args, before)
	}

	query += ` ORDER BY created_at ASC`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []domain.Message
	for rows.Next() {
		var msg domain.Message
		var runID, metadata sql.NullString
		if err := rows.Scan(&msg.MessageID, &msg.SessionID, &runID, &msg.Role, &msg.Content, &msg.CreatedAt, &metadata); err != nil {
			return nil, err
		}
		if runID.Valid {
			msg.RunID = runID.String
		}
		if metadata.Valid {
			msg.Metadata = json.RawMessage(metadata.String)
		}
		messages = append(messages, msg)
	}
	return messages, rows.Err()
}

// CreateRun creates a new run.
func (s *SQLiteStore) CreateRun(ctx context.Context, run *domain.Run) error {
	var parentRunID sql.NullString
	if run.ParentRunID != "" {
		parentRunID = sql.NullString{String: run.ParentRunID, Valid: true}
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO runs (run_id, session_id, root_agent_id, parent_run_id, status, started_at) VALUES (?, ?, ?, ?, ?, ?)`,
		run.RunID, run.SessionID, run.RootAgentID, parentRunID, run.Status, run.StartedAt)
	return err
}

// GetRun retrieves a run by ID.
func (s *SQLiteStore) GetRun(ctx context.Context, runID string) (*domain.Run, error) {
	var run domain.Run
	var parentRunID, errData sql.NullString
	var endedAt sql.NullTime
	err := s.db.QueryRowContext(ctx,
		`SELECT run_id, session_id, root_agent_id, parent_run_id, status, started_at, ended_at, error FROM runs WHERE run_id = ?`,
		runID).Scan(&run.RunID, &run.SessionID, &run.RootAgentID, &parentRunID, &run.Status, &run.StartedAt, &endedAt, &errData)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if parentRunID.Valid {
		run.ParentRunID = parentRunID.String
	}
	if endedAt.Valid {
		run.EndedAt = &endedAt.Time
	}
	if errData.Valid {
		run.Error = json.RawMessage(errData.String)
	}
	return &run, nil
}

// UpdateRunStatus updates the status of a run.
func (s *SQLiteStore) UpdateRunStatus(ctx context.Context, runID string, status domain.RunStatus) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE runs SET status = ? WHERE run_id = ?`,
		status, runID)
	return err
}

// UpdateRunCompleted updates a run to completed state.
func (s *SQLiteStore) UpdateRunCompleted(ctx context.Context, runID string, status domain.RunStatus, errData []byte) error {
	now := time.Now()
	var errStr sql.NullString
	if errData != nil {
		errStr = sql.NullString{String: string(errData), Valid: true}
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE runs SET status = ?, ended_at = ?, error = ? WHERE run_id = ?`,
		status, now, errStr, runID)
	return err
}

// CreateEvent creates a new event.
func (s *SQLiteStore) CreateEvent(ctx context.Context, event *domain.Event) error {
	payload := ""
	if event.Payload != nil {
		payload = string(event.Payload)
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO events (event_id, run_id, ts, type, payload) VALUES (?, ?, ?, ?, ?)`,
		event.EventID, event.RunID, event.Ts, event.Type, payload)
	return err
}

// GetEvents retrieves events for a run.
func (s *SQLiteStore) GetEvents(ctx context.Context, runID string, afterTs int64, types []string, limit int) ([]domain.Event, error) {
	query := `SELECT event_id, run_id, ts, type, payload FROM events WHERE run_id = ?`
	args := []interface{}{runID}

	if afterTs > 0 {
		query += ` AND ts > ?`
		args = append(args, afterTs)
	}

	if len(types) > 0 {
		placeholders := make([]string, len(types))
		for i, t := range types {
			placeholders[i] = "?"
			args = append(args, t)
		}
		query += fmt.Sprintf(" AND type IN (%s)", strings.Join(placeholders, ","))
	}

	query += ` ORDER BY ts ASC`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []domain.Event
	for rows.Next() {
		var event domain.Event
		var payload sql.NullString
		if err := rows.Scan(&event.EventID, &event.RunID, &event.Ts, &event.Type, &payload); err != nil {
			return nil, err
		}
		if payload.Valid {
			event.Payload = json.RawMessage(payload.String)
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

// RegisterAgent registers or updates an agent.
func (s *SQLiteStore) RegisterAgent(ctx context.Context, agent *domain.Agent) error {
	caps, _ := json.Marshal(agent.Capabilities)
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO agents (agent_id, name, endpoint, capabilities, status, last_heartbeat, created_at) 
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		agent.AgentID, agent.Name, agent.Endpoint, string(caps), agent.Status, agent.LastHeartbeat, agent.CreatedAt)
	return err
}

// GetAgent retrieves an agent by ID.
func (s *SQLiteStore) GetAgent(ctx context.Context, agentID string) (*domain.Agent, error) {
	var agent domain.Agent
	var caps sql.NullString
	var lastHeartbeat sql.NullTime
	err := s.db.QueryRowContext(ctx,
		`SELECT agent_id, name, endpoint, capabilities, status, last_heartbeat, created_at FROM agents WHERE agent_id = ?`,
		agentID).Scan(&agent.AgentID, &agent.Name, &agent.Endpoint, &caps, &agent.Status, &lastHeartbeat, &agent.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if caps.Valid {
		agent.Capabilities = json.RawMessage(caps.String)
	}
	if lastHeartbeat.Valid {
		agent.LastHeartbeat = &lastHeartbeat.Time
	}
	return &agent, nil
}

// ListAgents lists all agents.
func (s *SQLiteStore) ListAgents(ctx context.Context) ([]domain.Agent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT agent_id, name, endpoint, capabilities, status, last_heartbeat, created_at FROM agents ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []domain.Agent
	for rows.Next() {
		var agent domain.Agent
		var caps sql.NullString
		var lastHeartbeat sql.NullTime
		if err := rows.Scan(&agent.AgentID, &agent.Name, &agent.Endpoint, &caps, &agent.Status, &lastHeartbeat, &agent.CreatedAt); err != nil {
			return nil, err
		}
		if caps.Valid {
			agent.Capabilities = json.RawMessage(caps.String)
		}
		if lastHeartbeat.Valid {
			agent.LastHeartbeat = &lastHeartbeat.Time
		}
		agents = append(agents, agent)
	}
	return agents, rows.Err()
}
