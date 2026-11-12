package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"

	"github.com/jsortizq/politicas-iso27001-pyme/internal/auth"
	"github.com/jsortizq/politicas-iso27001-pyme/internal/database"
	"github.com/jsortizq/politicas-iso27001-pyme/internal/tenant"
)

// PostgreSQL stores. Every statement runs inside database.TenantTx, so Row-
// Level Security restricts it to the tenant even if a WHERE clause were
// wrong. Mutations append to audit_log in the same transaction: a change
// and its audit record commit or roll back together.

// pgErr translates driver errors into the store errors handlers understand.
func pgErr(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	var pqe *pq.Error
	if errors.As(err, &pqe) && pqe.Code == "23505" { // unique_violation
		return ErrConflict
	}
	return err
}

// validID guards queries against ids that are not UUIDs, which PostgreSQL
// would reject with a syntax error instead of "no rows".
func validID(id string) error {
	if !tenant.IsValidUUID(id) {
		return ErrNotFound
	}
	return nil
}

// audit appends an audit record within tx. The actor's e-mail comes from the
// authenticated session; "system" is used for unauthenticated callers.
func audit(ctx context.Context, tx *sql.Tx, tenantID, action, entityType, entityID string, detail map[string]any) error {
	if detail == nil {
		detail = map[string]any{}
	}
	detail["actor"] = "system"
	if u := auth.FromContext(ctx); u != nil {
		detail["actor"] = u.Email
		detail["role"] = u.Role
	}
	detail["origin"] = "api"
	body, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("audit: encode detail: %w", err)
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO audit_log (tenant_id, action, entity_type, entity_id, detail)
		 VALUES ($1, $2, $3, $4, $5)`,
		tenantID, action, entityType, entityID, string(body))
	if err != nil {
		return fmt.Errorf("audit: insert: %w", err)
	}
	return nil
}

func expectOneRow(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ── SoA ──

// PgSoAStore persists SoA entries in PostgreSQL.
type PgSoAStore struct {
	db *sql.DB
}

// NewPgSoAStore creates a store backed by db.
func NewPgSoAStore(db *sql.DB) *PgSoAStore { return &PgSoAStore{db: db} }

const soaColumns = `id, tenant_id, control_id, applicable, justification,
	implementation_pct, implementation_evidence, effectiveness_pct,
	effectiveness_evidence, responsible_role, review_date, created_at, updated_at`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSoA(row rowScanner) (SoAEntry, error) {
	var e SoAEntry
	var review sql.NullTime
	err := row.Scan(&e.ID, &e.TenantID, &e.ControlID, &e.Applicable, &e.Justification,
		&e.ImplementationPct, &e.ImplementationEv, &e.EffectivenessPct,
		&e.EffectivenessEv, &e.ResponsibleRole, &review, &e.CreatedAt, &e.UpdatedAt)
	if review.Valid {
		t := review.Time
		e.ReviewDate = &t
	}
	return e, err
}

// The schema keeps categorical statuses (ADR-004) next to the percentages;
// they are derived here so both views stay consistent.
func implementationStatus(pct int) string {
	switch {
	case pct >= 100:
		return "implemented"
	case pct > 0:
		return "partial"
	default:
		return "not_implemented"
	}
}

func effectivenessStatus(pct int) string {
	switch {
	case pct >= 80:
		return "effective"
	case pct >= 50:
		return "partially_effective"
	case pct > 0:
		return "ineffective"
	default:
		return "not_measured"
	}
}

func nullTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

func (s *PgSoAStore) List(ctx context.Context, tenantID string) ([]SoAEntry, error) {
	entries := []SoAEntry{}
	err := database.TenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT `+soaColumns+` FROM soa_entries WHERE tenant_id = $1`, tenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			e, err := scanSoA(rows)
			if err != nil {
				return err
			}
			entries = append(entries, e)
		}
		return rows.Err()
	})
	return entries, err
}

func (s *PgSoAStore) Get(ctx context.Context, tenantID, id string) (*SoAEntry, error) {
	if err := validID(id); err != nil {
		return nil, err
	}
	var e SoAEntry
	err := database.TenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		var err error
		e, err = scanSoA(tx.QueryRowContext(ctx,
			`SELECT `+soaColumns+` FROM soa_entries WHERE tenant_id = $1 AND id = $2`, tenantID, id))
		return pgErr(err)
	})
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (s *PgSoAStore) Create(ctx context.Context, e *SoAEntry) error {
	return database.TenantTx(ctx, s.db, e.TenantID, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
			INSERT INTO soa_entries (tenant_id, control_id, applicable, justification,
				implementation_status, implementation_pct, implementation_evidence,
				effectiveness, effectiveness_pct, effectiveness_evidence,
				responsible_role, review_date)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			RETURNING id, created_at, updated_at`,
			e.TenantID, e.ControlID, e.Applicable, e.Justification,
			implementationStatus(e.ImplementationPct), e.ImplementationPct, e.ImplementationEv,
			effectivenessStatus(e.EffectivenessPct), e.EffectivenessPct, e.EffectivenessEv,
			e.ResponsibleRole, nullTime(e.ReviewDate),
		).Scan(&e.ID, &e.CreatedAt, &e.UpdatedAt)
		if err != nil {
			return pgErr(err)
		}
		return audit(ctx, tx, e.TenantID, "soa.create", "soa_entry", e.ID,
			map[string]any{"control_id": e.ControlID, "applicable": e.Applicable})
	})
}

func (s *PgSoAStore) Update(ctx context.Context, e *SoAEntry) error {
	if err := validID(e.ID); err != nil {
		return err
	}
	return database.TenantTx(ctx, s.db, e.TenantID, func(tx *sql.Tx) error {
		// Lock the row and capture the previous assessment for the audit trail.
		var before struct {
			Applicable bool
			Impl, Eff  int
		}
		err := tx.QueryRowContext(ctx,
			`SELECT applicable, implementation_pct, effectiveness_pct
			 FROM soa_entries WHERE tenant_id = $1 AND id = $2 FOR UPDATE`,
			e.TenantID, e.ID).Scan(&before.Applicable, &before.Impl, &before.Eff)
		if err != nil {
			return pgErr(err)
		}

		err = tx.QueryRowContext(ctx, `
			UPDATE soa_entries SET
				control_id = $3, applicable = $4, justification = $5,
				implementation_status = $6, implementation_pct = $7, implementation_evidence = $8,
				effectiveness = $9, effectiveness_pct = $10, effectiveness_evidence = $11,
				responsible_role = $12, review_date = $13,
				last_effectiveness_review = CASE WHEN effectiveness_pct IS DISTINCT FROM $10
					THEN NOW() ELSE last_effectiveness_review END
			WHERE tenant_id = $1 AND id = $2
			RETURNING updated_at`,
			e.TenantID, e.ID, e.ControlID, e.Applicable, e.Justification,
			implementationStatus(e.ImplementationPct), e.ImplementationPct, e.ImplementationEv,
			effectivenessStatus(e.EffectivenessPct), e.EffectivenessPct, e.EffectivenessEv,
			e.ResponsibleRole, nullTime(e.ReviewDate),
		).Scan(&e.UpdatedAt)
		if err != nil {
			return pgErr(err)
		}
		return audit(ctx, tx, e.TenantID, "soa.update", "soa_entry", e.ID, map[string]any{
			"control_id": e.ControlID,
			"before": map[string]any{"applicable": before.Applicable,
				"implementation_pct": before.Impl, "effectiveness_pct": before.Eff},
			"after": map[string]any{"applicable": e.Applicable,
				"implementation_pct": e.ImplementationPct, "effectiveness_pct": e.EffectivenessPct},
		})
	})
}

func (s *PgSoAStore) Delete(ctx context.Context, tenantID, id string) error {
	if err := validID(id); err != nil {
		return err
	}
	return database.TenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM soa_entries WHERE tenant_id = $1 AND id = $2`, tenantID, id)
		if err != nil {
			return pgErr(err)
		}
		if err := expectOneRow(res); err != nil {
			return err
		}
		return audit(ctx, tx, tenantID, "soa.delete", "soa_entry", id, nil)
	})
}

// ── Assets ──

// PgAssetStore persists the asset inventory in PostgreSQL.
type PgAssetStore struct {
	db *sql.DB
}

// NewPgAssetStore creates a store backed by db.
func NewPgAssetStore(db *sql.DB) *PgAssetStore { return &PgAssetStore{db: db} }

const assetColumns = `id, tenant_id, name, asset_type, description, owner, location,
	classification, confidentiality, integrity, availability, criticality, status,
	created_at, updated_at`

func scanAsset(row rowScanner) (Asset, error) {
	var a Asset
	err := row.Scan(&a.ID, &a.TenantID, &a.Name, &a.Type, &a.Description, &a.Owner, &a.Location,
		&a.Classification, &a.Confidentiality, &a.Integrity, &a.Availability, &a.Criticality,
		&a.Status, &a.CreatedAt, &a.UpdatedAt)
	return a, err
}

func (s *PgAssetStore) List(ctx context.Context, tenantID string) ([]Asset, error) {
	assets := []Asset{}
	err := database.TenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT `+assetColumns+` FROM assets WHERE tenant_id = $1`, tenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			a, err := scanAsset(rows)
			if err != nil {
				return err
			}
			assets = append(assets, a)
		}
		return rows.Err()
	})
	return assets, err
}

func (s *PgAssetStore) Get(ctx context.Context, tenantID, id string) (*Asset, error) {
	if err := validID(id); err != nil {
		return nil, err
	}
	var a Asset
	err := database.TenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		var err error
		a, err = scanAsset(tx.QueryRowContext(ctx,
			`SELECT `+assetColumns+` FROM assets WHERE tenant_id = $1 AND id = $2`, tenantID, id))
		return pgErr(err)
	})
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *PgAssetStore) Create(ctx context.Context, a *Asset) error {
	return database.TenantTx(ctx, s.db, a.TenantID, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
			INSERT INTO assets (tenant_id, name, asset_type, description, owner, location,
				classification, confidentiality, integrity, availability, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			RETURNING id, criticality, created_at, updated_at`,
			a.TenantID, a.Name, a.Type, a.Description, a.Owner, a.Location,
			a.Classification, a.Confidentiality, a.Integrity, a.Availability, a.Status,
		).Scan(&a.ID, &a.Criticality, &a.CreatedAt, &a.UpdatedAt)
		if err != nil {
			return pgErr(err)
		}
		return audit(ctx, tx, a.TenantID, "asset.create", "asset", a.ID, map[string]any{"name": a.Name})
	})
}

func (s *PgAssetStore) Update(ctx context.Context, a *Asset) error {
	if err := validID(a.ID); err != nil {
		return err
	}
	return database.TenantTx(ctx, s.db, a.TenantID, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
			UPDATE assets SET name = $3, asset_type = $4, description = $5, owner = $6,
				location = $7, classification = $8, confidentiality = $9, integrity = $10,
				availability = $11, status = $12
			WHERE tenant_id = $1 AND id = $2
			RETURNING criticality, updated_at`,
			a.TenantID, a.ID, a.Name, a.Type, a.Description, a.Owner, a.Location,
			a.Classification, a.Confidentiality, a.Integrity, a.Availability, a.Status,
		).Scan(&a.Criticality, &a.UpdatedAt)
		if err != nil {
			return pgErr(err)
		}
		return audit(ctx, tx, a.TenantID, "asset.update", "asset", a.ID,
			map[string]any{"name": a.Name, "classification": a.Classification, "criticality": a.Criticality})
	})
}

func (s *PgAssetStore) Delete(ctx context.Context, tenantID, id string) error {
	if err := validID(id); err != nil {
		return err
	}
	return database.TenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM assets WHERE tenant_id = $1 AND id = $2`, tenantID, id)
		if err != nil {
			return pgErr(err)
		}
		if err := expectOneRow(res); err != nil {
			return err
		}
		return audit(ctx, tx, tenantID, "asset.delete", "asset", id, nil)
	})
}

// ── Risks ──

// PgRiskStore persists the risk register in PostgreSQL.
type PgRiskStore struct {
	db *sql.DB
}

// NewPgRiskStore creates a store backed by db.
func NewPgRiskStore(db *sql.DB) *PgRiskStore { return &PgRiskStore{db: db} }

const riskColumns = `id, tenant_id, code, title, description, asset, threat, vulnerability,
	likelihood, impact, treatment, treatment_plan, residual_likelihood, residual_impact,
	status, COALESCE(to_char(review_date, 'YYYY-MM-DD'), ''), created_at, updated_at`

func scanRisk(row rowScanner) (Risk, error) {
	var r Risk
	var resL, resI sql.NullInt64
	err := row.Scan(&r.ID, &r.TenantID, &r.Code, &r.Title, &r.Description, &r.Asset, &r.Threat,
		&r.Vulnerability, &r.Likelihood, &r.Impact, &r.Treatment, &r.TreatmentPlan, &resL, &resI,
		&r.Status, &r.ReviewDate, &r.CreatedAt, &r.UpdatedAt)
	if resL.Valid && resI.Valid {
		l, i := int(resL.Int64), int(resI.Int64)
		r.ResidualLikelihood, r.ResidualImpact = &l, &i
	}
	r.derive()
	return r, err
}

func nullInt(v *int) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*v), Valid: true}
}

func (s *PgRiskStore) List(ctx context.Context, tenantID string) ([]Risk, error) {
	risks := []Risk{}
	err := database.TenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT `+riskColumns+` FROM risks WHERE tenant_id = $1`, tenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			r, err := scanRisk(rows)
			if err != nil {
				return err
			}
			risks = append(risks, r)
		}
		return rows.Err()
	})
	return risks, err
}

func (s *PgRiskStore) Get(ctx context.Context, tenantID, id string) (*Risk, error) {
	if err := validID(id); err != nil {
		return nil, err
	}
	var r Risk
	err := database.TenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		var err error
		r, err = scanRisk(tx.QueryRowContext(ctx,
			`SELECT `+riskColumns+` FROM risks WHERE tenant_id = $1 AND id = $2`, tenantID, id))
		return pgErr(err)
	})
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *PgRiskStore) Create(ctx context.Context, r *Risk) error {
	return database.TenantTx(ctx, s.db, r.TenantID, func(tx *sql.Tx) error {
		if r.Code == "" {
			// Serialise code assignment per tenant so that two concurrent
			// creates cannot pick the same number.
			if _, err := tx.ExecContext(ctx,
				`SELECT pg_advisory_xact_lock(hashtext('risk-code:' || $1))`, r.TenantID); err != nil {
				return err
			}
			rows, err := tx.QueryContext(ctx, `SELECT code FROM risks WHERE tenant_id = $1`, r.TenantID)
			if err != nil {
				return err
			}
			var codes []string
			for rows.Next() {
				var c string
				if err := rows.Scan(&c); err != nil {
					rows.Close()
					return err
				}
				codes = append(codes, c)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}
			r.Code = nextRiskCode(codes)
		}

		err := tx.QueryRowContext(ctx, `
			INSERT INTO risks (tenant_id, code, title, description, asset, threat, vulnerability,
				likelihood, impact, treatment, treatment_plan, residual_likelihood, residual_impact,
				status, review_date)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, NULLIF($15, '')::date)
			RETURNING id, created_at, updated_at`,
			r.TenantID, r.Code, r.Title, r.Description, r.Asset, r.Threat, r.Vulnerability,
			r.Likelihood, r.Impact, r.Treatment, r.TreatmentPlan,
			nullInt(r.ResidualLikelihood), nullInt(r.ResidualImpact), r.Status, r.ReviewDate,
		).Scan(&r.ID, &r.CreatedAt, &r.UpdatedAt)
		if err != nil {
			return pgErr(err)
		}
		return audit(ctx, tx, r.TenantID, "risk.create", "risk", r.ID,
			map[string]any{"code": r.Code, "risk_level": r.Level, "treatment": r.Treatment})
	})
}

func (s *PgRiskStore) Update(ctx context.Context, r *Risk) error {
	if err := validID(r.ID); err != nil {
		return err
	}
	return database.TenantTx(ctx, s.db, r.TenantID, func(tx *sql.Tx) error {
		var before struct {
			Level             int
			Treatment, Status string
		}
		err := tx.QueryRowContext(ctx,
			`SELECT risk_level, treatment, status FROM risks WHERE tenant_id = $1 AND id = $2 FOR UPDATE`,
			r.TenantID, r.ID).Scan(&before.Level, &before.Treatment, &before.Status)
		if err != nil {
			return pgErr(err)
		}

		err = tx.QueryRowContext(ctx, `
			UPDATE risks SET code = $3, title = $4, description = $5, asset = $6, threat = $7,
				vulnerability = $8, likelihood = $9, impact = $10, treatment = $11,
				treatment_plan = $12, residual_likelihood = $13, residual_impact = $14,
				status = $15, review_date = NULLIF($16, '')::date
			WHERE tenant_id = $1 AND id = $2
			RETURNING updated_at`,
			r.TenantID, r.ID, r.Code, r.Title, r.Description, r.Asset, r.Threat, r.Vulnerability,
			r.Likelihood, r.Impact, r.Treatment, r.TreatmentPlan,
			nullInt(r.ResidualLikelihood), nullInt(r.ResidualImpact), r.Status, r.ReviewDate,
		).Scan(&r.UpdatedAt)
		if err != nil {
			return pgErr(err)
		}
		return audit(ctx, tx, r.TenantID, "risk.update", "risk", r.ID, map[string]any{
			"code": r.Code,
			"before": map[string]any{"risk_level": before.Level,
				"treatment": before.Treatment, "status": before.Status},
			"after": map[string]any{"risk_level": r.Level,
				"treatment": r.Treatment, "status": r.Status},
		})
	})
}

func (s *PgRiskStore) Delete(ctx context.Context, tenantID, id string) error {
	if err := validID(id); err != nil {
		return err
	}
	return database.TenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM risks WHERE tenant_id = $1 AND id = $2`, tenantID, id)
		if err != nil {
			return pgErr(err)
		}
		if err := expectOneRow(res); err != nil {
			return err
		}
		return audit(ctx, tx, tenantID, "risk.delete", "risk", id, nil)
	})
}
