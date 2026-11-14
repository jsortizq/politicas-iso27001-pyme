package api

import (
	"context"
	"database/sql"

	"github.com/lib/pq"

	"github.com/jsortizq/politicas-iso27001-pyme/internal/database"
	"github.com/jsortizq/politicas-iso27001-pyme/internal/tenant"
)

// nullUUID stores an id only when it is a UUID (member ids are; ids of
// synthetic users in tests may not be).
func nullUUID(id string) sql.NullString {
	if tenant.IsValidUUID(id) {
		return sql.NullString{String: id, Valid: true}
	}
	return sql.NullString{}
}

// replaceControls rewrites the control links of a document or evidence.
// table and column are constants chosen by the caller, never user input.
func replaceControls(ctx context.Context, tx *sql.Tx, table, column, tenantID, id string, controls []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE tenant_id = $1 AND `+column+` = $2`, tenantID, id); err != nil {
		return err
	}
	if len(controls) == 0 {
		return nil
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO `+table+` (tenant_id, `+column+`, control_id) SELECT $1, $2, unnest($3::text[])`,
		tenantID, id, pq.Array(controls))
	return err
}

// loadControls returns the control links of every record of the tenant in
// table, keyed by record id.
func loadControls(ctx context.Context, tx *sql.Tx, table, column, tenantID string) (map[string][]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT `+column+`, control_id FROM `+table+` WHERE tenant_id = $1`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	links := map[string][]string{}
	for rows.Next() {
		var id, control string
		if err := rows.Scan(&id, &control); err != nil {
			return nil, err
		}
		links[id] = append(links[id], control)
	}
	return links, rows.Err()
}

func sortControls(controls []string) []string {
	sorted, _ := normalizeControls(controls) // ids come from the catalog table
	return sorted
}

// ── Documents ──

// PgDocumentStore persists controlled documents in PostgreSQL.
type PgDocumentStore struct {
	db *sql.DB
}

// NewPgDocumentStore creates a store backed by db.
func NewPgDocumentStore(db *sql.DB) *PgDocumentStore { return &PgDocumentStore{db: db} }

const documentColumns = `id, tenant_id, title, doc_type, status, owner,
	COALESCE(to_char(next_review, 'YYYY-MM-DD'), ''), COALESCE(created_by::text, ''),
	created_by_email, created_at, updated_at`

func scanDocument(row rowScanner) (Document, error) {
	var d Document
	err := row.Scan(&d.ID, &d.TenantID, &d.Title, &d.Type, &d.Status, &d.Owner,
		&d.NextReview, &d.CreatedBy, &d.CreatedByEmail, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}

const versionColumns = `document_id, number, status, change_summary, file_name, mime_type,
	file_size, sha256, storage_key, COALESCE(uploaded_by::text, ''), uploaded_by_email, uploaded_at,
	COALESCE(reviewed_by::text, ''), reviewed_by_email, reviewed_at, review_note`

func scanVersion(row rowScanner) (string, DocumentVersion, error) {
	var docID string
	var v DocumentVersion
	var reviewedAt sql.NullTime
	err := row.Scan(&docID, &v.Number, &v.Status, &v.ChangeSummary, &v.Name, &v.MimeType,
		&v.Size, &v.SHA256, &v.StorageKey, &v.UploadedBy, &v.UploadedByEmail, &v.UploadedAt,
		&v.ReviewedBy, &v.ReviewedByEmail, &reviewedAt, &v.ReviewNote)
	if reviewedAt.Valid {
		v.ReviewedAt = &reviewedAt.Time
	}
	return docID, v, err
}

// loadVersions returns versions keyed by document id. With docID != ""
// only that document's versions are read.
func loadVersions(ctx context.Context, tx *sql.Tx, tenantID, docID string) (map[string][]DocumentVersion, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT `+versionColumns+` FROM document_versions
		WHERE tenant_id = $1 AND ($2 = '' OR document_id::text = $2)`, tenantID, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	versions := map[string][]DocumentVersion{}
	for rows.Next() {
		id, v, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		versions[id] = append(versions[id], v)
	}
	return versions, rows.Err()
}

func (s *PgDocumentStore) List(ctx context.Context, tenantID string) ([]Document, error) {
	docs := []Document{}
	err := database.TenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT `+documentColumns+` FROM documents WHERE tenant_id = $1`, tenantID)
		if err != nil {
			return err
		}
		for rows.Next() {
			d, err := scanDocument(rows)
			if err != nil {
				rows.Close()
				return err
			}
			docs = append(docs, d)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		versions, err := loadVersions(ctx, tx, tenantID, "")
		if err != nil {
			return err
		}
		controls, err := loadControls(ctx, tx, "document_controls", "document_id", tenantID)
		if err != nil {
			return err
		}
		for i := range docs {
			docs[i].Versions = versions[docs[i].ID]
			docs[i].Controls = sortControls(controls[docs[i].ID])
			docs[i].refresh()
		}
		return nil
	})
	return docs, err
}

// load reads one document with versions and controls inside tx.
func (s *PgDocumentStore) load(ctx context.Context, tx *sql.Tx, tenantID, id string, forUpdate bool) (*Document, error) {
	query := `SELECT ` + documentColumns + ` FROM documents WHERE tenant_id = $1 AND id = $2`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	d, err := scanDocument(tx.QueryRowContext(ctx, query, tenantID, id))
	if err != nil {
		return nil, pgErr(err)
	}
	versions, err := loadVersions(ctx, tx, tenantID, id)
	if err != nil {
		return nil, err
	}
	d.Versions = versions[id]

	rows, err := tx.QueryContext(ctx,
		`SELECT control_id FROM document_controls WHERE tenant_id = $1 AND document_id = $2`, tenantID, id)
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
	d.Controls = sortControls(controls)
	d.refresh()
	return &d, nil
}

func (s *PgDocumentStore) Get(ctx context.Context, tenantID, id string) (*Document, error) {
	if err := validID(id); err != nil {
		return nil, err
	}
	var doc *Document
	err := database.TenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		var err error
		doc, err = s.load(ctx, tx, tenantID, id, false)
		return err
	})
	return doc, err
}

func insertVersion(ctx context.Context, tx *sql.Tx, tenantID, docID string, v DocumentVersion) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO document_versions (tenant_id, document_id, number, status, change_summary,
			file_name, mime_type, file_size, sha256, storage_key,
			uploaded_by, uploaded_by_email, uploaded_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		tenantID, docID, v.Number, v.Status, v.ChangeSummary,
		v.Name, v.MimeType, v.Size, v.SHA256, v.StorageKey,
		nullUUID(v.UploadedBy), v.UploadedByEmail, v.UploadedAt)
	return pgErr(err)
}

func (s *PgDocumentStore) Create(ctx context.Context, doc *Document) error {
	doc.refresh()
	return database.TenantTx(ctx, s.db, doc.TenantID, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
			INSERT INTO documents (tenant_id, title, doc_type, status, owner, next_review,
				created_by, created_by_email)
			VALUES ($1, $2, $3, $4, $5, NULLIF($6, '')::date, $7, $8)
			RETURNING id, created_at, updated_at`,
			doc.TenantID, doc.Title, doc.Type, doc.Status, doc.Owner, doc.NextReview,
			nullUUID(doc.CreatedBy), doc.CreatedByEmail,
		).Scan(&doc.ID, &doc.CreatedAt, &doc.UpdatedAt)
		if err != nil {
			return pgErr(err)
		}
		for _, v := range doc.Versions {
			if err := insertVersion(ctx, tx, doc.TenantID, doc.ID, v); err != nil {
				return err
			}
		}
		if err := replaceControls(ctx, tx, "document_controls", "document_id", doc.TenantID, doc.ID, doc.Controls); err != nil {
			return pgErr(err)
		}
		first := doc.latest()
		return audit(ctx, tx, doc.TenantID, "document.create", "document", doc.ID, map[string]any{
			"title": doc.Title, "doc_type": doc.Type, "file_name": first.Name, "sha256": first.SHA256})
	})
}

func (s *PgDocumentStore) UpdateMeta(ctx context.Context, doc *Document) error {
	if err := validID(doc.ID); err != nil {
		return err
	}
	return database.TenantTx(ctx, s.db, doc.TenantID, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
			UPDATE documents SET title = $3, doc_type = $4, owner = $5, next_review = NULLIF($6, '')::date
			WHERE tenant_id = $1 AND id = $2
			RETURNING updated_at`,
			doc.TenantID, doc.ID, doc.Title, doc.Type, doc.Owner, doc.NextReview,
		).Scan(&doc.UpdatedAt)
		if err != nil {
			return pgErr(err)
		}
		if err := replaceControls(ctx, tx, "document_controls", "document_id", doc.TenantID, doc.ID, doc.Controls); err != nil {
			return pgErr(err)
		}
		return audit(ctx, tx, doc.TenantID, "document.update", "document", doc.ID,
			map[string]any{"title": doc.Title, "controls": doc.Controls})
	})
}

func (s *PgDocumentStore) Mutate(ctx context.Context, tenantID, id string, change func(d *Document) (AuditNote, error)) (*Document, error) {
	if err := validID(id); err != nil {
		return nil, err
	}
	var doc *Document
	err := database.TenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		// The row lock serialises concurrent changes to the same document.
		var err error
		doc, err = s.load(ctx, tx, tenantID, id, true)
		if err != nil {
			return err
		}
		known := map[int]bool{}
		for _, v := range doc.Versions {
			known[v.Number] = true
		}

		note, err := change(doc)
		if err != nil {
			return err
		}
		doc.refresh()

		// Save versions that left "approved" before the one that becomes
		// approved: at most one approved version may exist at any moment.
		ordered := append([]DocumentVersion(nil), doc.Versions...)
		for i, j := 0, 0; i < len(ordered); i++ {
			if ordered[i].Status != VersionApproved {
				ordered[i], ordered[j] = ordered[j], ordered[i]
				j++
			}
		}
		for _, v := range ordered {
			if !known[v.Number] {
				if err := insertVersion(ctx, tx, tenantID, id, v); err != nil {
					return err
				}
				continue
			}
			var reviewedAt sql.NullTime
			if v.ReviewedAt != nil {
				reviewedAt = sql.NullTime{Time: *v.ReviewedAt, Valid: true}
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE document_versions
				SET status = $4, reviewed_by = $5, reviewed_by_email = $6, reviewed_at = $7, review_note = $8
				WHERE tenant_id = $1 AND document_id = $2 AND number = $3
				  AND (status, reviewed_by_email, review_note) IS DISTINCT FROM ($4, $6, $8)`,
				tenantID, id, v.Number, v.Status, nullUUID(v.ReviewedBy), v.ReviewedByEmail, reviewedAt, v.ReviewNote,
			); err != nil {
				return pgErr(err)
			}
		}
		if err := tx.QueryRowContext(ctx,
			`UPDATE documents SET status = $3, updated_at = NOW() WHERE tenant_id = $1 AND id = $2 RETURNING updated_at`,
			tenantID, id, doc.Status).Scan(&doc.UpdatedAt); err != nil {
			return pgErr(err)
		}
		return audit(ctx, tx, tenantID, note.Action, "document", id, note.Detail)
	})
	if err != nil {
		return nil, err
	}
	return doc, nil
}

func (s *PgDocumentStore) Delete(ctx context.Context, tenantID, id string) ([]string, error) {
	if err := validID(id); err != nil {
		return nil, err
	}
	var keys []string
	err := database.TenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		doc, err := s.load(ctx, tx, tenantID, id, true)
		if err != nil {
			return err
		}
		// Checked on the locked row: a version approved meanwhile is kept.
		if err := doc.deletable(); err != nil {
			return err
		}
		hashes := make([]string, 0, len(doc.Versions))
		for _, v := range doc.Versions {
			keys = append(keys, v.StorageKey)
			hashes = append(hashes, v.SHA256)
		}
		// Versions and control links go with the document (ON DELETE CASCADE).
		res, err := tx.ExecContext(ctx, `DELETE FROM documents WHERE tenant_id = $1 AND id = $2`, tenantID, id)
		if err != nil {
			return pgErr(err)
		}
		if err := expectOneRow(res); err != nil {
			return err
		}
		return audit(ctx, tx, tenantID, "document.delete", "document", id,
			map[string]any{"title": doc.Title, "sha256": hashes})
	})
	if err != nil {
		return nil, err
	}
	return keys, nil
}

// ── Evidence ──

// PgEvidenceStore persists evidence records in PostgreSQL.
type PgEvidenceStore struct {
	db *sql.DB
}

// NewPgEvidenceStore creates a store backed by db.
func NewPgEvidenceStore(db *sql.DB) *PgEvidenceStore { return &PgEvidenceStore{db: db} }

const evidenceColumns = `id, tenant_id, title, description, to_char(collected_on, 'YYYY-MM-DD'),
	COALESCE(to_char(valid_until, 'YYYY-MM-DD'), ''), file_name, mime_type, file_size, sha256,
	storage_key, COALESCE(uploaded_by::text, ''), uploaded_by_email, created_at, updated_at`

func scanEvidence(row rowScanner) (Evidence, error) {
	var e Evidence
	err := row.Scan(&e.ID, &e.TenantID, &e.Title, &e.Description, &e.CollectedOn, &e.ValidUntil,
		&e.Name, &e.MimeType, &e.Size, &e.SHA256, &e.StorageKey,
		&e.UploadedBy, &e.UploadedByEmail, &e.CreatedAt, &e.UpdatedAt)
	return e, err
}

func (s *PgEvidenceStore) List(ctx context.Context, tenantID string) ([]Evidence, error) {
	items := []Evidence{}
	err := database.TenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT `+evidenceColumns+` FROM evidences WHERE tenant_id = $1`, tenantID)
		if err != nil {
			return err
		}
		for rows.Next() {
			e, err := scanEvidence(rows)
			if err != nil {
				rows.Close()
				return err
			}
			items = append(items, e)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		controls, err := loadControls(ctx, tx, "evidence_controls", "evidence_id", tenantID)
		if err != nil {
			return err
		}
		for i := range items {
			items[i].Controls = sortControls(controls[items[i].ID])
			items[i].refresh()
		}
		return nil
	})
	return items, err
}

func (s *PgEvidenceStore) Get(ctx context.Context, tenantID, id string) (*Evidence, error) {
	if err := validID(id); err != nil {
		return nil, err
	}
	var e Evidence
	err := database.TenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		var err error
		e, err = scanEvidence(tx.QueryRowContext(ctx,
			`SELECT `+evidenceColumns+` FROM evidences WHERE tenant_id = $1 AND id = $2`, tenantID, id))
		if err != nil {
			return pgErr(err)
		}
		rows, err := tx.QueryContext(ctx,
			`SELECT control_id FROM evidence_controls WHERE tenant_id = $1 AND evidence_id = $2`, tenantID, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		var controls []string
		for rows.Next() {
			var c string
			if err := rows.Scan(&c); err != nil {
				return err
			}
			controls = append(controls, c)
		}
		e.Controls = sortControls(controls)
		e.refresh()
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (s *PgEvidenceStore) Create(ctx context.Context, e *Evidence) error {
	return database.TenantTx(ctx, s.db, e.TenantID, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
			INSERT INTO evidences (tenant_id, title, description, collected_on, valid_until,
				file_name, mime_type, file_size, sha256, storage_key, uploaded_by, uploaded_by_email)
			VALUES ($1, $2, $3, $4::date, NULLIF($5, '')::date, $6, $7, $8, $9, $10, $11, $12)
			RETURNING id, created_at, updated_at`,
			e.TenantID, e.Title, e.Description, e.CollectedOn, e.ValidUntil,
			e.Name, e.MimeType, e.Size, e.SHA256, e.StorageKey, nullUUID(e.UploadedBy), e.UploadedByEmail,
		).Scan(&e.ID, &e.CreatedAt, &e.UpdatedAt)
		if err != nil {
			return pgErr(err)
		}
		if err := replaceControls(ctx, tx, "evidence_controls", "evidence_id", e.TenantID, e.ID, e.Controls); err != nil {
			return pgErr(err)
		}
		return audit(ctx, tx, e.TenantID, "evidence.create", "evidence", e.ID, map[string]any{
			"title": e.Title, "file_name": e.Name, "sha256": e.SHA256, "controls": e.Controls})
	})
}

func (s *PgEvidenceStore) Update(ctx context.Context, e *Evidence) error {
	if err := validID(e.ID); err != nil {
		return err
	}
	return database.TenantTx(ctx, s.db, e.TenantID, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
			UPDATE evidences SET title = $3, description = $4, collected_on = $5::date,
				valid_until = NULLIF($6, '')::date
			WHERE tenant_id = $1 AND id = $2
			RETURNING updated_at`,
			e.TenantID, e.ID, e.Title, e.Description, e.CollectedOn, e.ValidUntil,
		).Scan(&e.UpdatedAt)
		if err != nil {
			return pgErr(err)
		}
		if err := replaceControls(ctx, tx, "evidence_controls", "evidence_id", e.TenantID, e.ID, e.Controls); err != nil {
			return pgErr(err)
		}
		return audit(ctx, tx, e.TenantID, "evidence.update", "evidence", e.ID,
			map[string]any{"title": e.Title, "controls": e.Controls})
	})
}

func (s *PgEvidenceStore) Delete(ctx context.Context, tenantID, id string) (string, error) {
	if err := validID(id); err != nil {
		return "", err
	}
	var key string
	err := database.TenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		var title, hash string
		err := tx.QueryRowContext(ctx,
			`DELETE FROM evidences WHERE tenant_id = $1 AND id = $2 RETURNING storage_key, title, sha256`,
			tenantID, id).Scan(&key, &title, &hash)
		if err != nil {
			return pgErr(err)
		}
		// The hash stays in the audit trail: it proves which file existed.
		return audit(ctx, tx, tenantID, "evidence.delete", "evidence", id,
			map[string]any{"title": title, "sha256": hash})
	})
	if err != nil {
		return "", err
	}
	return key, nil
}
