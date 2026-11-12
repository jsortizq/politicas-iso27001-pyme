package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/jsortizq/politicas-iso27001-pyme/internal/auth"
	"github.com/jsortizq/politicas-iso27001-pyme/internal/database"
)

// AuditEvent is one record of the append-only audit trail.
type AuditEvent struct {
	ID         int64           `json:"id"`
	Action     string          `json:"action"`
	EntityType string          `json:"entity_type"`
	EntityID   string          `json:"entity_id,omitempty"`
	Actor      string          `json:"actor"`
	Detail     json.RawMessage `json:"detail"`
	CreatedAt  time.Time       `json:"created_at"`
}

// AuditStore reads the audit trail, newest first. before is an exclusive
// upper bound on the event id for paging (0 starts at the newest event).
type AuditStore interface {
	List(ctx context.Context, tenantID string, limit int, before int64) ([]AuditEvent, error)
}

// AuditHandler serves the audit trail to owners, administrators and auditors.
type AuditHandler struct {
	store  AuditStore // nil when the audit trail is not recorded (in-memory mode)
	logger *slog.Logger
}

// NewAuditHandler creates the handler. Pass a nil store when running
// without a database: the trail is only recorded in PostgreSQL.
func NewAuditHandler(store AuditStore, logger *slog.Logger) *AuditHandler {
	return &AuditHandler{store: store, logger: logger}
}

// Register wires the audit route into the given mux.
func (h *AuditHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/audit-log", h.handleList)
}

const (
	auditDefaultLimit = 50
	auditMaxLimit     = 200
)

func (h *AuditHandler) handleList(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if !user.CanViewAudit() {
		writeError(w, http.StatusForbidden, "your role cannot view the audit trail")
		return
	}
	if h.store == nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []AuditEvent{}, "recorded": false})
		return
	}

	limit := auditDefaultLimit
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = min(n, auditMaxLimit)
	}
	var before int64
	if v := r.URL.Query().Get("before"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "before must be a positive integer")
			return
		}
		before = n
	}

	// Fetch one extra row to know whether another page exists.
	events, err := h.store.List(r.Context(), user.TenantID, limit+1, before)
	if err != nil {
		h.logger.Error("audit list failed", "error", err, "tenant", user.TenantID)
		writeError(w, http.StatusInternalServerError, "failed to read the audit trail")
		return
	}
	resp := map[string]any{"recorded": true}
	if len(events) > limit {
		events = events[:limit]
		resp["next_before"] = events[len(events)-1].ID
	}
	resp["data"] = events
	writeJSON(w, http.StatusOK, resp)
}

// PgAuditStore reads audit_log from PostgreSQL.
type PgAuditStore struct {
	db *sql.DB
}

// NewPgAuditStore creates a store backed by db.
func NewPgAuditStore(db *sql.DB) *PgAuditStore { return &PgAuditStore{db: db} }

func (s *PgAuditStore) List(ctx context.Context, tenantID string, limit int, before int64) ([]AuditEvent, error) {
	events := []AuditEvent{}
	err := database.TenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
			SELECT id, action, entity_type, COALESCE(entity_id::text, ''),
			       COALESCE(detail->>'actor', ''), detail, created_at
			FROM audit_log
			WHERE tenant_id = $1 AND ($2 = 0 OR id < $2)
			ORDER BY id DESC
			LIMIT $3`, tenantID, before, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e AuditEvent
			var detail []byte
			if err := rows.Scan(&e.ID, &e.Action, &e.EntityType, &e.EntityID, &e.Actor, &detail, &e.CreatedAt); err != nil {
				return err
			}
			e.Detail = detail
			events = append(events, e)
		}
		return rows.Err()
	})
	return events, err
}
