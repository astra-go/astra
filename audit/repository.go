// Package audit provides structured audit logging for Astra applications.
//
// Audit logs capture security-relevant events such as logins, registrations,
// and administrative actions.  Each entry carries client metadata (IP, User-Agent,
// Device ID, Request ID) that is propagated through the request context.
//
// # Quick start
//
//	import "github.com/astra-go/astra/audit"
//
//	repo := audit.NewSQLRepository(db)       // PostgreSQL
//	// or: repo := audit.NewMemoryRepository()
//
//	// Log an event with automatic context propagation
//	audit.Log(ctx, repo, tenantID, &uin, audit.ActionLogin, audit.StatusSuccess, nil)
//
//	// Log with details
//	audit.Log(ctx, repo, tenantID, &uin, audit.ActionRegister, audit.StatusSuccess,
//	    audit.Details{"method": "email"})
//
// # Context propagation
//
// Client metadata is attached to the context via middleware or transport handler:
//
//	ctx := audit.WithClientMetadata(ctx, ip, userAgent, deviceID, requestID)
//
// The metadata is then automatically extracted when audit.Log is called,
// so callers don't need to pass IP/UA/RequestID explicitly.
package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Status represents the outcome of an audited action.
type Status string

const (
	StatusSuccess Status = "SUCCESS"
	StatusFailure Status = "FAILURE"
)

// Action represents a type of auditable operation.
type Action string

// Common audit actions. Applications may define additional actions.
const (
	ActionLogin        Action = "LOGIN"
	ActionRegister     Action = "REGISTER"
	ActionLogout       Action = "LOGOUT"
	ActionSendCode     Action = "SEND_CODE"
	ActionVerifyCode   Action = "VERIFY_CODE"
	ActionRefreshToken Action = "REFRESH_TOKEN"
	ActionVerifyToken  Action = "VERIFY_TOKEN"
	ActionResetPwd     Action = "RESET_PASSWORD"
	ActionChangePwd    Action = "CHANGE_PASSWORD"
	ActionBindPhone    Action = "BIND_PHONE"
	ActionUnbindPhone  Action = "UNBIND_PHONE"
	ActionBindEmail    Action = "BIND_EMAIL"
	ActionUnbindEmail  Action = "UNBIND_EMAIL"
	ActionBindSocial   Action = "BIND_SOCIAL"
	ActionUnbindSocial Action = "UNBIND_SOCIAL"
	ActionDeviceReport Action = "DEVICE_REPORT"
	ActionRevokeTokens Action = "REVOKE_TOKENS"
	ActionAdminOp      Action = "ADMIN_OPERATION"
)

// Details is a convenience alias for event metadata.
type Details map[string]any

// Entry represents a single audit log record.
type Entry struct {
	ID        int64
	TenantID  int64
	UIN       *int64
	Action    Action
	Status    Status
	IPAddress string
	UserAgent string
	DeviceID  string
	RequestID string
	Details   Details
	CreatedAt time.Time
}

// Repository defines the persistence interface for audit logs.
type Repository interface {
	// Log persists an audit entry. Implementations may choose asynchronous writes.
	Log(ctx context.Context, entry Entry) error

	// List returns audit entries for a tenant, ordered by created_at DESC.
	// Pass an empty action to list all actions.
	List(ctx context.Context, tenantID int64, action Action, limit, offset int) ([]Entry, error)
}

// SQLRepository stores audit logs in PostgreSQL.
type SQLRepository struct {
	db *sql.DB
}

// NewSQLRepository creates a PostgreSQL-backed audit repository.
// Panics when db is nil.
func NewSQLRepository(db *sql.DB) *SQLRepository {
	if db == nil {
		panic("audit.NewSQLRepository: db is required")
	}
	return &SQLRepository{db: db}
}

// Log inserts an audit entry synchronously.
func (r *SQLRepository) Log(ctx context.Context, entry Entry) error {
	detailsBytes := []byte("{}")
	if len(entry.Details) > 0 {
		b, err := json.Marshal(entry.Details)
		if err != nil {
			return fmt.Errorf("marshal audit details: %w", err)
		}
		detailsBytes = b
	}

	var uin sql.NullInt64
	if entry.UIN != nil {
		uin.Int64 = *entry.UIN
		uin.Valid = true
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO audit_logs (tenant_id, uin, action, status, ip_address, user_agent, device_id, request_id, details, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, entry.TenantID, uin, entry.Action, entry.Status, entry.IPAddress, entry.UserAgent, entry.DeviceID, entry.RequestID, detailsBytes, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("insert audit log: %w", err)
	}
	return nil
}

// List returns audit entries for a tenant, optionally filtered by action.
func (r *SQLRepository) List(ctx context.Context, tenantID int64, action Action, limit, offset int) ([]Entry, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	var rows *sql.Rows
	var err error
	if action != "" {
		rows, err = r.db.QueryContext(ctx, `
			SELECT id, tenant_id, uin, action, status, ip_address, user_agent, device_id, request_id, details, created_at
			FROM audit_logs
			WHERE tenant_id = $1 AND action = $2
			ORDER BY created_at DESC
			LIMIT $3 OFFSET $4
		`, tenantID, action, limit, offset)
	} else {
		rows, err = r.db.QueryContext(ctx, `
			SELECT id, tenant_id, uin, action, status, ip_address, user_agent, device_id, request_id, details, created_at
			FROM audit_logs
			WHERE tenant_id = $1
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3
		`, tenantID, limit, offset)
	}
	if err != nil {
		return nil, fmt.Errorf("query audit logs: %w", err)
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var e Entry
		var uin sql.NullInt64
		var detailsBytes []byte
		if err := rows.Scan(&e.ID, &e.TenantID, &uin, &e.Action, &e.Status, &e.IPAddress, &e.UserAgent, &e.DeviceID, &e.RequestID, &detailsBytes, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan audit log: %w", err)
		}
		if uin.Valid {
			v := uin.Int64
			e.UIN = &v
		}
		if len(detailsBytes) > 0 {
			_ = json.Unmarshal(detailsBytes, &e.Details)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// MemoryRepository is an in-memory audit repository for development and testing.
type MemoryRepository struct {
	entries []Entry
	lastID  int64
}

// NewMemoryRepository creates an in-memory audit repository.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{}
}

// Log appends an entry to the in-memory store.
func (r *MemoryRepository) Log(_ context.Context, entry Entry) error {
	r.lastID++
	entry.ID = r.lastID
	entry.CreatedAt = time.Now().UTC()
	r.entries = append(r.entries, entry)
	return nil
}

// List returns entries in reverse insertion order, filtered by tenant and optionally action.
func (r *MemoryRepository) List(_ context.Context, tenantID int64, action Action, limit, offset int) ([]Entry, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	var filtered []Entry
	for i := len(r.entries) - 1; i >= 0; i-- {
		e := r.entries[i]
		if e.TenantID != tenantID {
			continue
		}
		if action != "" && e.Action != action {
			continue
		}
		filtered = append(filtered, e)
	}

	if offset >= len(filtered) {
		return []Entry{}, nil
	}
	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[offset:end], nil
}

// compile-time interface checks
var _ Repository = (*SQLRepository)(nil)
var _ Repository = (*MemoryRepository)(nil)
