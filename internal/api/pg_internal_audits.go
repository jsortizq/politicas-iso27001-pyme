package api

import (
	"context"
	"database/sql"

	"github.com/jsortizq/politicas-iso27001-pyme/internal/database"
)

// PgInternalAuditStore persists audits and findings in PostgreSQL.
type PgInternalAuditStore struct {
	db *sql.DB
}

// NewPgInternalAuditStore creates a store backed by db.
func NewPgInternalAuditStore(db *sql.DB) *PgInternalAuditStore { return &PgInternalAuditStore{db: db} }

const internalAuditColumns = `id, tenant_id, title, audit_type, status, scope, criteria, lead_auditor,
	COALESCE(to_char(planned_date, 'YYYY-MM-DD'), ''), COALESCE(to_char(completed_date, 'YYYY-MM-DD'), ''),
	summary, cancel_reason, created_by_email, created_at, updated_at`

func scanInternalAudit(row rowScanner) (InternalAudit, error) {
	var a InternalAudit
	err := row.Scan(&a.ID, &a.TenantID, &a.Title, &a.Type, &a.Status, &a.Scope, &a.Criteria, &a.LeadAuditor,
		&a.PlannedDate, &a.CompletedDate, &a.Summary, &a.CancelReason, &a.CreatedByEmail, &a.CreatedAt, &a.UpdatedAt)
	return a, err
}

const findingColumns = `audit_id, id, number, severity, COALESCE(control_id, ''), clause, description, evidence,
	root_cause, corrective_action, responsible, COALESCE(to_char(due_date, 'YYYY-MM-DD'), ''), status,
	verification, closed_by_email, closed_at, reopen_note, created_by_email, created_at, updated_at`

func scanFinding(row rowScanner) (string, Finding, error) {
	var auditID string
	var f Finding
	var closedAt sql.NullTime
	err := row.Scan(&auditID, &f.ID, &f.Number, &f.Severity, &f.Control, &f.Clause, &f.Description, &f.Evidence,
		&f.RootCause, &f.CorrectiveAction, &f.Responsible, &f.DueDate, &f.Status,
		&f.Verification, &f.ClosedByEmail, &closedAt, &f.ReopenNote, &f.CreatedByEmail, &f.CreatedAt, &f.UpdatedAt)
	if closedAt.Valid {
		f.ClosedAt = &closedAt.Time
	}
	return auditID, f, err
}

// loadFindings returns findings keyed by audit id. With auditID != "" only
// that audit's findings are read.
func loadFindings(ctx context.Context, tx *sql.Tx, tenantID, auditID string) (map[string][]Finding, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT `+findingColumns+` FROM audit_findings
		WHERE tenant_id = $1 AND ($2 = '' OR audit_id::text = $2)`, tenantID, auditID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	findings := map[string][]Finding{}
	for rows.Next() {
		id, f, err := scanFinding(rows)
		if err != nil {
			return nil, err
		}
		findings[id] = append(findings[id], f)
	}
	return findings, rows.Err()
}

func (s *PgInternalAuditStore) List(ctx context.Context, tenantID string) ([]InternalAudit, error) {
	audits := []InternalAudit{}
	err := database.TenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT `+internalAuditColumns+` FROM audits WHERE tenant_id = $1`, tenantID)
		if err != nil {
			return err
		}
		for rows.Next() {
			a, err := scanInternalAudit(rows)
			if err != nil {
				rows.Close()
				return err
			}
			audits = append(audits, a)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		findings, err := loadFindings(ctx, tx, tenantID, "")
		if err != nil {
			return err
		}
		controls, err := loadControls(ctx, tx, "audit_controls", "audit_id", tenantID)
		if err != nil {
			return err
		}
		for i := range audits {
			audits[i].Findings = findings[audits[i].ID]
			audits[i].Controls = sortControls(controls[audits[i].ID])
			audits[i].refresh()
		}
		return nil
	})
	return audits, err
}

func (s *PgInternalAuditStore) load(ctx context.Context, tx *sql.Tx, tenantID, id string, forUpdate bool) (*InternalAudit, error) {
	query := `SELECT ` + internalAuditColumns + ` FROM audits WHERE tenant_id = $1 AND id = $2`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	a, err := scanInternalAudit(tx.QueryRowContext(ctx, query, tenantID, id))
	if err != nil {
		return nil, pgErr(err)
	}
	findings, err := loadFindings(ctx, tx, tenantID, id)
	if err != nil {
		return nil, err
	}
	a.Findings = findings[id]

	rows, err := tx.QueryContext(ctx,
		`SELECT control_id FROM audit_controls WHERE tenant_id = $1 AND audit_id = $2`, tenantID, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var controls []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		controls = append(controls, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	a.Controls = sortControls(controls)
	a.refresh()
	return &a, nil
}

func (s *PgInternalAuditStore) Get(ctx context.Context, tenantID, id string) (*InternalAudit, error) {
	if err := validID(id); err != nil {
		return nil, err
	}
	var a *InternalAudit
	err := database.TenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		var err error
		a, err = s.load(ctx, tx, tenantID, id, false)
		return err
	})
	return a, err
}

func (s *PgInternalAuditStore) Create(ctx context.Context, a *InternalAudit) error {
	return database.TenantTx(ctx, s.db, a.TenantID, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
			INSERT INTO audits (tenant_id, title, audit_type, status, scope, criteria, lead_auditor,
				planned_date, created_by_email)
			VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, '')::date, $9)
			RETURNING id, created_at, updated_at`,
			a.TenantID, a.Title, a.Type, a.Status, a.Scope, a.Criteria, a.LeadAuditor,
			a.PlannedDate, a.CreatedByEmail,
		).Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
		if err != nil {
			return pgErr(err)
		}
		if err := replaceControls(ctx, tx, "audit_controls", "audit_id", a.TenantID, a.ID, a.Controls); err != nil {
			return pgErr(err)
		}
		return audit(ctx, tx, a.TenantID, "audit.create", "audit", a.ID,
			map[string]any{"title": a.Title, "audit_type": a.Type, "planned_date": a.PlannedDate})
	})
}

func (s *PgInternalAuditStore) Mutate(ctx context.Context, tenantID, id string, change func(a *InternalAudit) (AuditNote, error)) (*InternalAudit, error) {
	if err := validID(id); err != nil {
		return nil, err
	}
	var a *InternalAudit
	err := database.TenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		// The row lock serialises concurrent changes to the same audit,
		// including the numbering of new findings.
		var err error
		a, err = s.load(ctx, tx, tenantID, id, true)
		if err != nil {
			return err
		}
		before := map[string]Finding{}
		for _, f := range a.Findings {
			before[f.ID] = f
		}

		note, err := change(a)
		if err != nil {
			return err
		}
		a.refresh()

		if err := tx.QueryRowContext(ctx, `
			UPDATE audits SET title = $3, audit_type = $4, status = $5, scope = $6, criteria = $7,
				lead_auditor = $8, planned_date = NULLIF($9, '')::date,
				completed_date = NULLIF($10, '')::date, summary = $11, cancel_reason = $12,
				updated_at = NOW()
			WHERE tenant_id = $1 AND id = $2
			RETURNING updated_at`,
			tenantID, id, a.Title, a.Type, a.Status, a.Scope, a.Criteria, a.LeadAuditor,
			a.PlannedDate, a.CompletedDate, a.Summary, a.CancelReason,
		).Scan(&a.UpdatedAt); err != nil {
			return pgErr(err)
		}
		if err := replaceControls(ctx, tx, "audit_controls", "audit_id", tenantID, id, a.Controls); err != nil {
			return pgErr(err)
		}

		after := map[string]bool{}
		for _, f := range a.Findings {
			after[f.ID] = true
			var closedAt sql.NullTime
			if f.ClosedAt != nil {
				closedAt = sql.NullTime{Time: *f.ClosedAt, Valid: true}
			}
			previous, existed := before[f.ID]
			if existed && previous == f {
				continue // untouched: leave the row (and its updated_at) alone
			}
			if !existed {
				_, err = tx.ExecContext(ctx, `
					INSERT INTO audit_findings (id, tenant_id, audit_id, number, severity, control_id, clause,
						description, evidence, root_cause, corrective_action, responsible, due_date, status,
						verification, closed_by_email, closed_at, reopen_note, created_by_email)
					VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7, $8, $9, $10, $11, $12,
						NULLIF($13, '')::date, $14, $15, $16, $17, $18, $19)`,
					f.ID, tenantID, id, f.Number, f.Severity, f.Control, f.Clause,
					f.Description, f.Evidence, f.RootCause, f.CorrectiveAction, f.Responsible, f.DueDate, f.Status,
					f.Verification, f.ClosedByEmail, closedAt, f.ReopenNote, f.CreatedByEmail)
			} else {
				_, err = tx.ExecContext(ctx, `
					UPDATE audit_findings SET severity = $4, control_id = NULLIF($5, ''), clause = $6,
						description = $7, evidence = $8, root_cause = $9, corrective_action = $10,
						responsible = $11, due_date = NULLIF($12, '')::date, status = $13,
						verification = $14, closed_by_email = $15, closed_at = $16, reopen_note = $17
					WHERE tenant_id = $1 AND audit_id = $2 AND id = $3`,
					tenantID, id, f.ID, f.Severity, f.Control, f.Clause,
					f.Description, f.Evidence, f.RootCause, f.CorrectiveAction,
					f.Responsible, f.DueDate, f.Status,
					f.Verification, f.ClosedByEmail, closedAt, f.ReopenNote)
			}
			if err != nil {
				return pgErr(err)
			}
		}
		for findingID := range before {
			if !after[findingID] {
				if _, err := tx.ExecContext(ctx,
					`DELETE FROM audit_findings WHERE tenant_id = $1 AND audit_id = $2 AND id = $3`,
					tenantID, id, findingID); err != nil {
					return pgErr(err)
				}
			}
		}
		return audit(ctx, tx, tenantID, note.Action, "audit", id, note.Detail)
	})
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (s *PgInternalAuditStore) Delete(ctx context.Context, tenantID, id string) error {
	if err := validID(id); err != nil {
		return err
	}
	return database.TenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		// The row is locked so the status checked is the status deleted: an
		// audit started meanwhile keeps its findings.
		existing, err := s.load(ctx, tx, tenantID, id, true)
		if err != nil {
			return err
		}
		if err := existing.deletable(); err != nil {
			return err
		}
		title := existing.Title
		// Findings and control links go with the audit (ON DELETE CASCADE).
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM audits WHERE tenant_id = $1 AND id = $2`, tenantID, id); err != nil {
			return pgErr(err)
		}
		return audit(ctx, tx, tenantID, "audit.delete", "audit", id, map[string]any{"title": title})
	})
}
