package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/xiaot623/gogo/orchestrator/internal/domain"
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
	// For in-memory SQLite, multiple connections create separate databases.
	// Keep a single connection to avoid schema/data disappearing across goroutines.
	if dsn == ":memory:" || strings.Contains(dsn, "mode=memory") {
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
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

	// Seed tools
	if err := store.seedTools(); err != nil {
		fmt.Printf("Failed to seed tools: %v\n", err)
		// Don't fail startup for this
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
		// New tables
		`CREATE TABLE IF NOT EXISTS tools (
			name TEXT PRIMARY KEY,
			kind TEXT NOT NULL,
			schema TEXT,
			client_id TEXT,
			policy TEXT,
			timeout_ms INTEGER NOT NULL DEFAULT 60000,
			metadata TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_tools_client ON tools(client_id)`,
		`CREATE TABLE IF NOT EXISTS tool_calls (
			tool_call_id TEXT PRIMARY KEY,
			run_id TEXT NOT NULL,
			tool_name TEXT NOT NULL,
			kind TEXT NOT NULL,
			status TEXT NOT NULL,
			args TEXT,
			result TEXT,
			error TEXT,
			approval_id TEXT,
			idempotency_key TEXT,
			timeout_ms INTEGER NOT NULL DEFAULT 60000,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			completed_at DATETIME,
			FOREIGN KEY (run_id) REFERENCES runs(run_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_tool_calls_run ON tool_calls(run_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tool_calls_status_created ON tool_calls(status, created_at)`,
		`CREATE TABLE IF NOT EXISTS approvals (
			approval_id TEXT PRIMARY KEY,
			run_id TEXT NOT NULL,
			tool_call_id TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'PENDING',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			decided_at DATETIME,
			decided_by TEXT,
			reason TEXT,
			FOREIGN KEY (run_id) REFERENCES runs(run_id),
			FOREIGN KEY (tool_call_id) REFERENCES tool_calls(tool_call_id)
		)`,
	}

	for _, m := range migrations {
		if _, err := s.db.Exec(m); err != nil {
			return fmt.Errorf("migration failed: %w\n%s", err, m)
		}
	}

	// Add new columns for existing DBs (SQLite has limited ALTER TABLE support).
	if err := s.ensureColumn("tool_calls", "timeout_ms", "ALTER TABLE tool_calls ADD COLUMN timeout_ms INTEGER NOT NULL DEFAULT 60000"); err != nil {
		return err
	}
	if err := s.ensureColumn("tool_calls", "idempotency_key", "ALTER TABLE tool_calls ADD COLUMN idempotency_key TEXT"); err != nil {
		return err
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_tool_calls_idempotency ON tool_calls(run_id, tool_name, idempotency_key, created_at)`); err != nil {
		return err
	}

	return nil
}

func (s *SQLiteStore) ensureColumn(tableName, columnName, ddl string) error {
	rows, err := s.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dfltValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return err
		}
		if name == columnName {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	_, err = s.db.Exec(ddl)
	return err
}

func (s *SQLiteStore) seedTools() error {
	ctx := context.Background()
	tools := []domain.Tool{
		{
			Name:      "weather.query",
			Kind:      domain.ToolKindServer,
			TimeoutMs: 5000,
		},
		{
			Name:      "browser.screenshot",
			Kind:      domain.ToolKindClient,
			TimeoutMs: 60000,
		},
		{
			Name:      "payments.transfer",
			Kind:      domain.ToolKindServer,
			TimeoutMs: 10000,
		},
		{
			Name:      "dangerous.command",
			Kind:      domain.ToolKindServer,
			TimeoutMs: 5000,
		},
	}

	for _, t := range tools {
		if err := s.CreateTool(ctx, &t); err != nil {
			// Ignore if exists
			if !strings.Contains(err.Error(), "UNIQUE constraint failed") {
				return err
			}
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

// CreateTool creates a new tool.
func (s *SQLiteStore) CreateTool(ctx context.Context, tool *domain.Tool) error {
	schema, _ := json.Marshal(tool.Schema)
	policy, _ := json.Marshal(tool.Policy)
	metadata, _ := json.Marshal(tool.Metadata)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO tools (name, kind, schema, client_id, policy, timeout_ms, metadata) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		tool.Name, tool.Kind, string(schema), tool.ClientID, string(policy), tool.TimeoutMs, string(metadata))
	return err
}

// UpsertTool creates or updates a tool.
func (s *SQLiteStore) UpsertTool(ctx context.Context, tool *domain.Tool) error {
	schema, _ := json.Marshal(tool.Schema)
	policy, _ := json.Marshal(tool.Policy)
	metadata, _ := json.Marshal(tool.Metadata)
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO tools (name, kind, schema, client_id, policy, timeout_ms, metadata) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		tool.Name, tool.Kind, string(schema), tool.ClientID, string(policy), tool.TimeoutMs, string(metadata))
	return err
}

// GetTool retrieves a tool by name.
func (s *SQLiteStore) GetTool(ctx context.Context, toolName string) (*domain.Tool, error) {
	var tool domain.Tool
	var schema, clientID, policy, metadata sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT name, kind, schema, client_id, policy, timeout_ms, metadata FROM tools WHERE name = ?`,
		toolName).Scan(&tool.Name, &tool.Kind, &schema, &clientID, &policy, &tool.TimeoutMs, &metadata)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if schema.Valid {
		tool.Schema = json.RawMessage(schema.String)
	}
	if clientID.Valid {
		tool.ClientID = clientID.String
	}
	if policy.Valid {
		tool.Policy = json.RawMessage(policy.String)
	}
	if metadata.Valid {
		tool.Metadata = json.RawMessage(metadata.String)
	}
	return &tool, nil
}

// ListTools lists all tools.
func (s *SQLiteStore) ListTools(ctx context.Context) ([]domain.Tool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name, kind, schema, client_id, policy, timeout_ms, metadata FROM tools`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tools []domain.Tool
	for rows.Next() {
		var tool domain.Tool
		var schema, clientID, policy, metadata sql.NullString
		if err := rows.Scan(&tool.Name, &tool.Kind, &schema, &clientID, &policy, &tool.TimeoutMs, &metadata); err != nil {
			return nil, err
		}
		if schema.Valid {
			tool.Schema = json.RawMessage(schema.String)
		}
		if clientID.Valid {
			tool.ClientID = clientID.String
		}
		if policy.Valid {
			tool.Policy = json.RawMessage(policy.String)
		}
		if metadata.Valid {
			tool.Metadata = json.RawMessage(metadata.String)
		}
		tools = append(tools, tool)
	}
	return tools, rows.Err()
}

// CreateToolCall creates a new tool call.
func (s *SQLiteStore) CreateToolCall(ctx context.Context, toolCall *domain.ToolCall) error {
	args, _ := json.Marshal(toolCall.Args)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO tool_calls (tool_call_id, run_id, tool_name, kind, status, args, result, error, approval_id, idempotency_key, timeout_ms, created_at, completed_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		toolCall.ToolCallID, toolCall.RunID, toolCall.ToolName, toolCall.Kind, toolCall.Status, string(args), nullStringBytes(toolCall.Result), nullStringBytes(toolCall.Error), nullString(toolCall.ApprovalID), nullString(toolCall.IdempotencyKey), toolCall.TimeoutMs, toolCall.CreatedAt, toolCall.CompletedAt)
	return err
}

// GetToolCall retrieves a tool call by ID.
func (s *SQLiteStore) GetToolCall(ctx context.Context, toolCallID string) (*domain.ToolCall, error) {
	var tc domain.ToolCall
	var args, result, errData, approvalID, idempotencyKey sql.NullString
	var completedAt sql.NullTime

	err := s.db.QueryRowContext(ctx,
		`SELECT tool_call_id, run_id, tool_name, kind, status, args, result, error, approval_id, idempotency_key, timeout_ms, created_at, completed_at FROM tool_calls WHERE tool_call_id = ?`,
		toolCallID).Scan(&tc.ToolCallID, &tc.RunID, &tc.ToolName, &tc.Kind, &tc.Status, &args, &result, &errData, &approvalID, &idempotencyKey, &tc.TimeoutMs, &tc.CreatedAt, &completedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if args.Valid {
		tc.Args = json.RawMessage(args.String)
	}
	if result.Valid {
		tc.Result = json.RawMessage(result.String)
	}
	if errData.Valid {
		tc.Error = json.RawMessage(errData.String)
	}
	if approvalID.Valid {
		tc.ApprovalID = approvalID.String
	}
	if idempotencyKey.Valid {
		tc.IdempotencyKey = idempotencyKey.String
	}
	if completedAt.Valid {
		tc.CompletedAt = &completedAt.Time
	}
	return &tc, nil
}

// GetToolCallByIdempotencyKey retrieves the most recent tool call by idempotency key within a run.
func (s *SQLiteStore) GetToolCallByIdempotencyKey(ctx context.Context, runID string, toolName string, idempotencyKey string) (*domain.ToolCall, error) {
	var tc domain.ToolCall
	var args, result, errData, approvalID, idemKey sql.NullString
	var completedAt sql.NullTime

	err := s.db.QueryRowContext(ctx,
		`SELECT tool_call_id, run_id, tool_name, kind, status, args, result, error, approval_id, idempotency_key, timeout_ms, created_at, completed_at
		 FROM tool_calls
		 WHERE run_id = ? AND tool_name = ? AND idempotency_key = ?
		 ORDER BY created_at DESC
		 LIMIT 1`,
		runID, toolName, idempotencyKey).Scan(&tc.ToolCallID, &tc.RunID, &tc.ToolName, &tc.Kind, &tc.Status, &args, &result, &errData, &approvalID, &idemKey, &tc.TimeoutMs, &tc.CreatedAt, &completedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if args.Valid {
		tc.Args = json.RawMessage(args.String)
	}
	if result.Valid {
		tc.Result = json.RawMessage(result.String)
	}
	if errData.Valid {
		tc.Error = json.RawMessage(errData.String)
	}
	if approvalID.Valid {
		tc.ApprovalID = approvalID.String
	}
	if idemKey.Valid {
		tc.IdempotencyKey = idemKey.String
	}
	if completedAt.Valid {
		tc.CompletedAt = &completedAt.Time
	}
	return &tc, nil
}

// UpdateToolCallStatus updates the status of a tool call.
func (s *SQLiteStore) UpdateToolCallStatus(ctx context.Context, toolCallID string, status domain.ToolCallStatus) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE tool_calls SET status = ? WHERE tool_call_id = ? AND completed_at IS NULL`,
		status, toolCallID)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// UpdateToolCallResult updates the result of a tool call.
func (s *SQLiteStore) UpdateToolCallResult(ctx context.Context, toolCallID string, status domain.ToolCallStatus, result []byte, errData []byte) (bool, error) {
	now := time.Now()
	var resStr, errStr sql.NullString
	if result != nil {
		resStr = sql.NullString{String: string(result), Valid: true}
	}
	if errData != nil {
		errStr = sql.NullString{String: string(errData), Valid: true}
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE tool_calls SET status = ?, result = ?, error = ?, completed_at = ? WHERE tool_call_id = ? AND completed_at IS NULL`,
		status, resStr, errStr, now, toolCallID)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// UpdateToolCallApproval updates the approval ID and status of a tool call.
func (s *SQLiteStore) UpdateToolCallApproval(ctx context.Context, toolCallID string, approvalID string, status domain.ToolCallStatus) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE tool_calls SET approval_id = ?, status = ? WHERE tool_call_id = ? AND completed_at IS NULL`,
		approvalID, status, toolCallID)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (s *SQLiteStore) ListExpiredToolCalls(ctx context.Context, limit int) ([]domain.ToolCall, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT tool_call_id, run_id, tool_name, kind, status, args, approval_id, timeout_ms, created_at
		FROM tool_calls
		WHERE completed_at IS NULL
		  AND status NOT IN ('SUCCEEDED', 'FAILED', 'TIMEOUT', 'BLOCKED', 'REJECTED')
		  AND ((julianday('now') - julianday(created_at)) * 86400000.0) >= timeout_ms
		ORDER BY created_at ASC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.ToolCall
	for rows.Next() {
		var tc domain.ToolCall
		var args sql.NullString
		var approvalID sql.NullString
		if err := rows.Scan(&tc.ToolCallID, &tc.RunID, &tc.ToolName, &tc.Kind, &tc.Status, &args, &approvalID, &tc.TimeoutMs, &tc.CreatedAt); err != nil {
			return nil, err
		}
		if args.Valid {
			tc.Args = json.RawMessage(args.String)
		}
		if approvalID.Valid {
			tc.ApprovalID = approvalID.String
		}
		out = append(out, tc)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateApproval creates a new approval.
func (s *SQLiteStore) CreateApproval(ctx context.Context, approval *domain.Approval) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO approvals (approval_id, run_id, tool_call_id, status, created_at) VALUES (?, ?, ?, ?, ?)`,
		approval.ApprovalID, approval.RunID, approval.ToolCallID, approval.Status, approval.CreatedAt)
	return err
}

// GetApproval retrieves an approval by ID.
func (s *SQLiteStore) GetApproval(ctx context.Context, approvalID string) (*domain.Approval, error) {
	var ap domain.Approval
	var decidedAt sql.NullTime
	var decidedBy, reason sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT approval_id, run_id, tool_call_id, status, created_at, decided_at, decided_by, reason FROM approvals WHERE approval_id = ?`,
		approvalID).Scan(&ap.ApprovalID, &ap.RunID, &ap.ToolCallID, &ap.Status, &ap.CreatedAt, &decidedAt, &decidedBy, &reason)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if decidedAt.Valid {
		ap.DecidedAt = &decidedAt.Time
	}
	if decidedBy.Valid {
		ap.DecidedBy = decidedBy.String
	}
	if reason.Valid {
		ap.Reason = reason.String
	}
	return &ap, nil
}

// UpdateApprovalStatus updates the status of an approval.
func (s *SQLiteStore) UpdateApprovalStatus(ctx context.Context, approvalID string, status domain.ApprovalStatus, decidedBy string, reason string) error {
	now := time.Now()
	_, err := s.db.ExecContext(ctx,
		`UPDATE approvals SET status = ?, decided_at = ?, decided_by = ?, reason = ? WHERE approval_id = ?`,
		status, now, decidedBy, reason, approvalID)
	return err
}

func (s *SQLiteStore) ExpireApprovalIfPending(ctx context.Context, approvalID string, reason string) (bool, error) {
	now := time.Now()
	res, err := s.db.ExecContext(ctx,
		`UPDATE approvals SET status = ?, decided_at = ?, reason = ? WHERE approval_id = ? AND status = ?`,
		domain.ApprovalStatusExpired, now, reason, approvalID, domain.ApprovalStatusPending)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func nullStringBytes(b []byte) sql.NullString {
	if len(b) == 0 {
		return sql.NullString{}
	}
	return sql.NullString{String: string(b), Valid: true}
}
