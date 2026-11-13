package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/jsortizq/politicas-iso27001-pyme/internal/auth"
	"github.com/jsortizq/politicas-iso27001-pyme/internal/database/dbtest"
	"github.com/jsortizq/politicas-iso27001-pyme/internal/directory"
	"github.com/jsortizq/politicas-iso27001-pyme/internal/storage"
)

// fileEnv is the documents and evidence API over one backend, with real
// members of two organizations.
type fileEnv struct {
	t       *testing.T
	mux     *http.ServeMux
	root    string // storage directory
	store   storage.Store
	dbs     *dbtest.DBs // nil on the in-memory backend
	members map[string]*auth.User
}

const testMaxUpload = 1 << 20

// eachFileBackend runs test against the in-memory stores and, when
// TEST_DATABASE_URL is set, against PostgreSQL through the RLS-restricted role.
func eachFileBackend(t *testing.T, test func(t *testing.T, e *fileEnv)) {
	t.Run("memory", func(t *testing.T) { test(t, newFileEnv(t, nil)) })
	t.Run("postgres", func(t *testing.T) {
		dbs := dbtest.New(t) // skips without TEST_DATABASE_URL
		test(t, newFileEnv(t, &dbs))
	})
}

func newFileEnv(t *testing.T, dbs *dbtest.DBs) *fileEnv {
	t.Helper()
	root := t.TempDir()
	store, err := storage.NewDir(root)
	if err != nil {
		t.Fatal(err)
	}
	e := &fileEnv{t: t, mux: http.NewServeMux(), root: root, store: store, dbs: dbs, members: map[string]*auth.User{}}

	var (
		members directory.Store    = directory.NewMemoryStore()
		docs    DocumentStore      = NewMemoryDocumentStore()
		evid    EvidenceStore      = NewMemoryEvidenceStore()
		audits  InternalAuditStore = NewMemoryInternalAuditStore()
	)
	if dbs != nil {
		members = directory.NewPgStore(dbs.App)
		docs = NewPgDocumentStore(dbs.App)
		evid = NewPgEvidenceStore(dbs.App)
		audits = NewPgInternalAuditStore(dbs.App)
	}
	// Real member records, so uploaded_by / reviewed_by reference users.
	for _, m := range []struct{ key, tenant, role string }{
		{"owner", pgTenantA, auth.RoleOwner}, {"admin", pgTenantA, auth.RoleAdmin},
		{"editor", pgTenantA, auth.RoleEditor}, {"auditor", pgTenantA, auth.RoleAuditor},
		{"viewer", pgTenantA, auth.RoleViewer}, {"outsider", pgTenantB, auth.RoleOwner},
	} {
		membership, err := members.EnsureDevUser(context.Background(), m.tenant, m.key+"@empresa.test", m.key, m.role)
		if err != nil {
			t.Fatal(err)
		}
		e.members[m.key] = &auth.User{ID: membership.UserID, Email: membership.Email, TenantID: m.tenant, Role: m.role}
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	files := NewFiles(store, testMaxUpload, logger)
	NewDocumentHandler(docs, files, logger).Register(e.mux)
	NewEvidenceHandler(evid, files, logger).Register(e.mux)
	NewInternalAuditHandler(audits, logger).Register(e.mux)
	return e
}

// as performs a request as the named member. body may be a string (JSON),
// a *form (multipart) or nil.
func (e *fileEnv) as(member, method, path string, body any) *httptest.ResponseRecorder {
	e.t.Helper()
	var req *http.Request
	switch b := body.(type) {
	case nil:
		req = httptest.NewRequest(method, path, nil)
	case string:
		req = httptest.NewRequest(method, path, strings.NewReader(b))
	case *form:
		req = httptest.NewRequest(method, path, bytes.NewReader(b.body.Bytes()))
		req.Header.Set("Content-Type", b.contentType)
	default:
		e.t.Fatalf("unsupported body %T", body)
	}
	user := e.members[member]
	rr := httptest.NewRecorder()
	e.mux.ServeHTTP(rr, req.WithContext(auth.WithUser(req.Context(), user)))
	return rr
}

// storedFiles lists the objects currently in storage.
func (e *fileEnv) storedFiles() []string {
	var files []string
	filepath.WalkDir(e.root, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	return files
}

type form struct {
	body        bytes.Buffer
	contentType string
}

// formWith builds a multipart form. A nil content omits the file part.
func formWith(fields map[string]string, fileName string, content []byte) *form {
	f := &form{}
	w := multipart.NewWriter(&f.body)
	for k, v := range fields {
		w.WriteField(k, v)
	}
	if content != nil {
		part, _ := w.CreateFormFile("file", fileName)
		part.Write(content)
	}
	w.Close()
	f.contentType = w.FormDataContentType()
	return f
}

func mustStatus(t *testing.T, rr *httptest.ResponseRecorder, want int, what string) {
	t.Helper()
	if rr.Code != want {
		t.Fatalf("%s: expected %d, got %d: %s", what, want, rr.Code, rr.Body.String())
	}
}

func statuses(d Document) string {
	var parts []string
	for _, v := range d.Versions {
		parts = append(parts, fmt.Sprintf("v%d=%s", v.Number, v.Status))
	}
	return strings.Join(parts, " ")
}

var policyV1 = []byte("Política de seguridad de la información — versión 1")

func (e *fileEnv) newDocument(title string) Document {
	e.t.Helper()
	rr := e.as("editor", http.MethodPost, "/v1/documents",
		formWith(map[string]string{"title": title, "doc_type": "policy", "controls": "A.5.2, A.5.1"}, "politica.pdf", policyV1))
	mustStatus(e.t, rr, http.StatusCreated, "create document")
	return decode[Document](e.t, rr)
}

// ── Documents ──

func TestDocument_ApprovalLifecycle(t *testing.T) {
	eachFileBackend(t, func(t *testing.T, e *fileEnv) {
		doc := e.newDocument("Política de seguridad")
		if doc.Status != DocumentDraft || statuses(doc) != "v1=draft" || doc.ApprovedNumber != 0 {
			t.Fatalf("new document: %s / %s", doc.Status, statuses(doc))
		}
		v1 := doc.Versions[0]
		if v1.SHA256 != hashOf(policyV1) || v1.Size != int64(len(policyV1)) || v1.Name != "politica.pdf" ||
			v1.MimeType != "application/pdf" || v1.UploadedByEmail != "editor@empresa.test" {
			t.Errorf("version metadata: %+v", v1)
		}
		if strings.Join(doc.Controls, ",") != "A.5.1,A.5.2" {
			t.Errorf("controls should be sorted and trimmed: %v", doc.Controls)
		}
		base := "/v1/documents/" + doc.ID

		// Submit, then only owners and admins may approve.
		doc = decode[Document](t, e.as("editor", http.MethodPost, base+"/versions/1/submit", nil))
		if doc.Status != DocumentReview {
			t.Fatalf("after submit: %s", doc.Status)
		}
		mustStatus(t, e.as("editor", http.MethodPost, base+"/versions/1/approve", nil), http.StatusForbidden, "editor approving")
		rr := e.as("admin", http.MethodPost, base+"/versions/1/approve", `{"note":"Revisada por la dirección"}`)
		mustStatus(t, rr, http.StatusOK, "approve v1")
		doc = decode[Document](t, rr)
		if doc.Status != DocumentApproved || doc.ApprovedNumber != 1 || doc.Versions[0].ReviewedByEmail != "admin@empresa.test" ||
			doc.Versions[0].ReviewedAt == nil || doc.Versions[0].ReviewNote != "Revisada por la dirección" {
			t.Fatalf("after approval: %+v", doc)
		}

		// A new version does not displace the one in force until approved.
		v2 := []byte("versión 2 con cambios")
		rr = e.as("editor", http.MethodPost, base+"/versions", formWith(map[string]string{"change_summary": "Ajuste de alcance"}, "politica-v2.pdf", v2))
		mustStatus(t, rr, http.StatusCreated, "add v2")
		doc = decode[Document](t, rr)
		if statuses(doc) != "v2=draft v1=approved" || doc.Status != DocumentApproved || doc.ApprovedNumber != 1 {
			t.Fatalf("after v2 upload: %s (%s)", statuses(doc), doc.Status)
		}

		// Rejection needs a reason and keeps v1 in force.
		e.as("editor", http.MethodPost, base+"/versions/2/submit", nil)
		mustStatus(t, e.as("owner", http.MethodPost, base+"/versions/2/reject", `{"note":"  "}`), http.StatusUnprocessableEntity, "reject without reason")
		doc = decode[Document](t, e.as("owner", http.MethodPost, base+"/versions/2/reject", `{"note":"Falta el anexo de roles"}`))
		if statuses(doc) != "v2=rejected v1=approved" || doc.ApprovedNumber != 1 {
			t.Fatalf("after rejection: %s", statuses(doc))
		}

		// v3 is approved and supersedes v1.
		v3 := []byte("versión 3 corregida")
		e.as("editor", http.MethodPost, base+"/versions", formWith(nil, "politica-v3.pdf", v3))
		e.as("editor", http.MethodPost, base+"/versions/3/submit", nil)
		doc = decode[Document](t, e.as("owner", http.MethodPost, base+"/versions/3/approve", nil))
		if statuses(doc) != "v3=approved v2=rejected v1=superseded" || doc.ApprovedNumber != 3 {
			t.Fatalf("after v3 approval: %s", statuses(doc))
		}

		// Every version stays downloadable, byte for byte.
		for number, want := range map[int][]byte{1: policyV1, 2: v2, 3: v3} {
			rr := e.as("viewer", http.MethodGet, fmt.Sprintf("%s/versions/%d/download", base, number), nil)
			if rr.Code != http.StatusOK || !bytes.Equal(rr.Body.Bytes(), want) {
				t.Errorf("download v%d: %d, %d bytes", number, rr.Code, rr.Body.Len())
			}
		}
		integrity := decode[Integrity](t, e.as("auditor", http.MethodGet, base+"/versions/3/integrity", nil))
		if !integrity.Intact || integrity.ActualSHA256 != hashOf(v3) {
			t.Errorf("integrity: %+v", integrity)
		}

		// The stored document matches what the transitions returned.
		got := decode[Document](t, e.as("viewer", http.MethodGet, base, nil))
		if statuses(got) != statuses(doc) || got.Status != DocumentApproved || got.Versions[0].ReviewedByEmail != "owner@empresa.test" {
			t.Errorf("persisted document differs: %s", statuses(got))
		}
	})
}

func TestDocument_OnlyTheNewestVersionMoves(t *testing.T) {
	eachFileBackend(t, func(t *testing.T, e *fileEnv) {
		doc := e.newDocument("Procedimiento de respaldo")
		base := "/v1/documents/" + doc.ID

		mustStatus(t, e.as("admin", http.MethodPost, base+"/versions/1/approve", nil), http.StatusConflict, "approving a draft")
		mustStatus(t, e.as("admin", http.MethodPost, base+"/versions/1/reject", `{"note":"x"}`), http.StatusConflict, "rejecting a draft")

		// Uploading v2 supersedes the pending v1.
		doc = decode[Document](t, e.as("editor", http.MethodPost, base+"/versions", formWith(nil, "v2.pdf", []byte("v2"))))
		if statuses(doc) != "v2=draft v1=superseded" {
			t.Fatalf("after v2: %s", statuses(doc))
		}
		mustStatus(t, e.as("editor", http.MethodPost, base+"/versions/1/submit", nil), http.StatusConflict, "submitting an old version")
		mustStatus(t, e.as("editor", http.MethodPost, base+"/versions/9/submit", nil), http.StatusNotFound, "unknown version")
		mustStatus(t, e.as("editor", http.MethodPost, base+"/versions/x/submit", nil), http.StatusNotFound, "malformed version")

		mustStatus(t, e.as("editor", http.MethodPost, base+"/versions/2/submit", nil), http.StatusOK, "submit v2")
		mustStatus(t, e.as("editor", http.MethodPost, base+"/versions/2/submit", nil), http.StatusConflict, "submitting twice")
		mustStatus(t, e.as("admin", http.MethodPost, base+"/versions/2/approve", nil), http.StatusOK, "approve v2")
		mustStatus(t, e.as("admin", http.MethodPost, base+"/versions/2/approve", nil), http.StatusConflict, "approving twice")
		mustStatus(t, e.as("admin", http.MethodPost, base+"/versions/2/reject", `{"note":"x"}`), http.StatusConflict, "rejecting an approved version")
	})
}

func TestDocument_ObsoleteAndDelete(t *testing.T) {
	eachFileBackend(t, func(t *testing.T, e *fileEnv) {
		// A document that was approved is history: it can be withdrawn, not deleted.
		approved := e.newDocument("Política de accesos")
		base := "/v1/documents/" + approved.ID
		e.as("editor", http.MethodPost, base+"/versions/1/submit", nil)
		e.as("admin", http.MethodPost, base+"/versions/1/approve", nil)

		mustStatus(t, e.as("admin", http.MethodDelete, base, nil), http.StatusConflict, "deleting an approved document")
		mustStatus(t, e.as("editor", http.MethodPost, base+"/obsolete", nil), http.StatusForbidden, "editor withdrawing")
		doc := decode[Document](t, e.as("admin", http.MethodPost, base+"/obsolete", nil))
		if doc.Status != DocumentObsolete {
			t.Fatalf("after obsolete: %s", doc.Status)
		}
		// Nothing changes afterwards, but it can still be read.
		for name, rr := range map[string]*httptest.ResponseRecorder{
			"obsolete twice": e.as("admin", http.MethodPost, base+"/obsolete", nil),
			"new version":    e.as("editor", http.MethodPost, base+"/versions", formWith(nil, "x.pdf", []byte("x"))),
			"edit":           e.as("editor", http.MethodPut, base, `{"title":"Otro","doc_type":"policy"}`),
			"delete":         e.as("owner", http.MethodDelete, base, nil),
		} {
			mustStatus(t, rr, http.StatusConflict, name+" on an obsolete document")
		}
		mustStatus(t, e.as("viewer", http.MethodGet, base+"/versions/1/download", nil), http.StatusOK, "download of an obsolete document")
		if n := len(e.storedFiles()); n != 1 {
			t.Errorf("refused uploads must not leave files: %d stored", n)
		}

		// A document never approved can be deleted, files included.
		draft := e.newDocument("Borrador descartable")
		e.as("editor", http.MethodPost, "/v1/documents/"+draft.ID+"/versions", formWith(nil, "v2.pdf", []byte("v2")))
		if n := len(e.storedFiles()); n != 3 {
			t.Fatalf("expected 3 stored files, got %d", n)
		}
		mustStatus(t, e.as("editor", http.MethodDelete, "/v1/documents/"+draft.ID, nil), http.StatusForbidden, "editor deleting")
		mustStatus(t, e.as("admin", http.MethodDelete, "/v1/documents/"+draft.ID, nil), http.StatusNoContent, "delete draft")
		mustStatus(t, e.as("admin", http.MethodGet, "/v1/documents/"+draft.ID, nil), http.StatusNotFound, "get deleted")
		if n := len(e.storedFiles()); n != 1 {
			t.Errorf("deleting a document should remove its 2 files: %d stored", n)
		}
	})
}

func TestDocument_UpdateFilterAndReadOnlyRoles(t *testing.T) {
	eachFileBackend(t, func(t *testing.T, e *fileEnv) {
		doc := e.newDocument("Política de seguridad")
		other := e.newDocument("Inventario de activos")
		base := "/v1/documents/" + doc.ID

		rr := e.as("editor", http.MethodPut, "/v1/documents/"+other.ID,
			`{"title":" Inventario de activos 2026 ","doc_type":"record","owner":"TI","next_review":"2020-01-01","controls":["A.5.9","A.5.9"]}`)
		mustStatus(t, rr, http.StatusOK, "update")
		updated := decode[Document](t, rr)
		if updated.Title != "Inventario de activos 2026" || updated.Type != "record" || updated.Owner != "TI" ||
			strings.Join(updated.Controls, ",") != "A.5.9" || len(updated.Versions) != 1 {
			t.Errorf("update: %+v", updated)
		}
		// A review date in the past only counts as overdue once a version is in force.
		if updated.ReviewOverdue {
			t.Error("a draft cannot be overdue for review")
		}
		e.as("editor", http.MethodPost, "/v1/documents/"+other.ID+"/versions/1/submit", nil)
		if got := decode[Document](t, e.as("admin", http.MethodPost, "/v1/documents/"+other.ID+"/versions/1/approve", nil)); !got.ReviewOverdue {
			t.Error("an approved document past its review date should be overdue")
		}

		list := decode[struct {
			Count int
			Data  []Document
		}](t, e.as("viewer", http.MethodGet, "/v1/documents", nil))
		if list.Count != 2 || list.Data[0].Title != "Inventario de activos 2026" || len(list.Data[1].Versions) != 1 {
			t.Errorf("list: %+v", list)
		}
		for control, want := range map[string]int{"A.5.1": 1, "A.5.9": 1, "A.8.1": 0} {
			got := decode[struct{ Count int }](t, e.as("viewer", http.MethodGet, "/v1/documents?control="+control, nil))
			if got.Count != want {
				t.Errorf("filter %s: got %d, want %d", control, got.Count, want)
			}
		}

		// Read-only roles read and download, nothing else.
		for _, role := range []string{"viewer", "auditor"} {
			mustStatus(t, e.as(role, http.MethodGet, base, nil), http.StatusOK, role+" get")
			mustStatus(t, e.as(role, http.MethodGet, base+"/versions/1/download", nil), http.StatusOK, role+" download")
			for name, rr := range map[string]*httptest.ResponseRecorder{
				"create":      e.as(role, http.MethodPost, "/v1/documents", formWith(map[string]string{"title": "x", "doc_type": "policy"}, "x.pdf", []byte("x"))),
				"edit":        e.as(role, http.MethodPut, base, `{"title":"x","doc_type":"policy"}`),
				"new version": e.as(role, http.MethodPost, base+"/versions", formWith(nil, "x.pdf", []byte("x"))),
				"submit":      e.as(role, http.MethodPost, base+"/versions/1/submit", nil),
				"approve":     e.as(role, http.MethodPost, base+"/versions/1/approve", nil),
				"obsolete":    e.as(role, http.MethodPost, base+"/obsolete", nil),
				"delete":      e.as(role, http.MethodDelete, base, nil),
			} {
				mustStatus(t, rr, http.StatusForbidden, role+" "+name)
			}
		}
		if n := len(e.storedFiles()); n != 2 {
			t.Errorf("forbidden uploads must not leave files: %d stored", n)
		}
	})
}

func TestDocument_UploadValidation(t *testing.T) {
	eachFileBackend(t, func(t *testing.T, e *fileEnv) {
		ok := map[string]string{"title": "Política", "doc_type": "policy"}
		with := func(k, v string) map[string]string {
			m := map[string]string{k: v}
			for key, val := range ok {
				if _, set := m[key]; !set {
					m[key] = val
				}
			}
			return m
		}
		cases := []struct {
			name string
			body any
			want int
		}{
			{"no file", formWith(ok, "", nil), http.StatusUnprocessableEntity},
			{"empty file", formWith(ok, "vacio.pdf", []byte{}), http.StatusUnprocessableEntity},
			{"no title", formWith(with("title", "  "), "p.pdf", []byte("x")), http.StatusUnprocessableEntity},
			{"bad type", formWith(with("doc_type", "evidence"), "p.pdf", []byte("x")), http.StatusUnprocessableEntity},
			{"bad date", formWith(with("next_review", "31/12/2026"), "p.pdf", []byte("x")), http.StatusUnprocessableEntity},
			{"unknown control", formWith(with("controls", "A.5.1,A.9.99"), "p.pdf", []byte("x")), http.StatusUnprocessableEntity},
			{"not multipart", `{"title":"Política","doc_type":"policy"}`, http.StatusBadRequest},
			{"too large", formWith(ok, "grande.bin", bytes.Repeat([]byte("x"), testMaxUpload+1)), http.StatusRequestEntityTooLarge},
		}
		for _, c := range cases {
			mustStatus(t, e.as("editor", http.MethodPost, "/v1/documents", c.body), c.want, c.name)
		}
		if files := e.storedFiles(); len(files) != 0 {
			t.Errorf("rejected uploads left files in storage: %v", files)
		}
		if got := decode[struct{ Count int }](t, e.as("editor", http.MethodGet, "/v1/documents", nil)); got.Count != 0 {
			t.Errorf("rejected uploads created %d documents", got.Count)
		}

		// A file of exactly the maximum size is accepted.
		rr := e.as("editor", http.MethodPost, "/v1/documents", formWith(ok, "justo.bin", bytes.Repeat([]byte("x"), testMaxUpload)))
		mustStatus(t, rr, http.StatusCreated, "file at the size limit")
	})
}

func TestDownload_IsAlwaysAnInertAttachment(t *testing.T) {
	e := newFileEnv(t, nil)
	html := []byte(`<html><script>alert(document.cookie)</script></html>`)
	rr := e.as("editor", http.MethodPost, "/v1/evidence", formWith(nil, "../../evil página.html", html))
	mustStatus(t, rr, http.StatusCreated, "upload")
	ev := decode[Evidence](t, rr)
	if ev.Name != "evil página.html" {
		t.Errorf("file name should lose its path: %q", ev.Name)
	}

	rr = e.as("viewer", http.MethodGet, "/v1/evidence/"+ev.ID+"/download", nil)
	h := rr.Header()
	if h.Get("Content-Type") != "application/octet-stream" || h.Get("X-Content-Type-Options") != "nosniff" ||
		!strings.HasPrefix(h.Get("Content-Disposition"), "attachment;") ||
		!strings.Contains(h.Get("Content-Security-Policy"), "sandbox") || h.Get("Cache-Control") != "private, no-store" {
		t.Errorf("download headers: %v", h)
	}
	if !strings.Contains(h.Get("Content-Disposition"), "p%C3%A1gina.html") {
		t.Errorf("non-ASCII file names should be RFC 2231 encoded: %s", h.Get("Content-Disposition"))
	}
	if !bytes.Equal(rr.Body.Bytes(), html) {
		t.Error("content was altered")
	}
}

func TestCleanFileName(t *testing.T) {
	for in, want := range map[string]string{
		"informe.pdf":                  "informe.pdf",
		"/etc/passwd":                  "passwd",
		`C:\Users\ana\Acta junta.docx`: "Acta junta.docx",
		"../../x":                      "x",
		"a\x00b\r\n.txt":               "ab.txt",
		"  ":                           "archivo",
		"..":                           "archivo",
		"":                             "archivo",
	} {
		if got := cleanFileName(in); got != want {
			t.Errorf("cleanFileName(%q) = %q, want %q", in, got, want)
		}
	}
	long := cleanFileName(strings.Repeat("ñ", 300) + ".pdf")
	if len([]rune(long)) != 200 || !strings.HasSuffix(long, ".pdf") {
		t.Errorf("long names should be cut to 200 characters keeping the extension: %d %q", len([]rune(long)), long[len(long)-8:])
	}
}

// ── Evidence ──

func TestEvidence_Lifecycle(t *testing.T) {
	eachFileBackend(t, func(t *testing.T, e *fileEnv) {
		content := []byte("captura del panel de respaldos")
		rr := e.as("editor", http.MethodPost, "/v1/evidence", formWith(map[string]string{
			"description": "Respaldo diario verificado", "collected_on": "2020-01-15",
			"valid_until": "2020-07-15", "controls": "A.8.13",
		}, "respaldos-enero.png", content))
		mustStatus(t, rr, http.StatusCreated, "create")
		ev := decode[Evidence](t, rr)
		// Without a title the file name is used; a past validity date marks it expired.
		if ev.Title != "respaldos-enero.png" || ev.SHA256 != hashOf(content) || ev.MimeType != "image/png" ||
			!ev.Expired || ev.UploadedByEmail != "editor@empresa.test" || strings.Join(ev.Controls, ",") != "A.8.13" {
			t.Fatalf("created evidence: %+v", ev)
		}
		base := "/v1/evidence/" + ev.ID

		rr = e.as("editor", http.MethodPut, base,
			`{"title":"Respaldos de enero","description":"","collected_on":"2020-01-15","valid_until":"","controls":["A.8.13","A.5.30"]}`)
		mustStatus(t, rr, http.StatusOK, "update")
		ev = decode[Evidence](t, rr)
		if ev.Title != "Respaldos de enero" || ev.Expired || strings.Join(ev.Controls, ",") != "A.5.30,A.8.13" || ev.SHA256 != hashOf(content) {
			t.Errorf("updated evidence: %+v", ev)
		}

		got := decode[Evidence](t, e.as("viewer", http.MethodGet, base, nil))
		if got.Title != "Respaldos de enero" || got.Name != "respaldos-enero.png" || len(got.Controls) != 2 {
			t.Errorf("persisted evidence: %+v", got)
		}
		for control, want := range map[string]int{"A.8.13": 1, "A.5.30": 1, "A.5.1": 0} {
			if list := decode[struct{ Count int }](t, e.as("viewer", http.MethodGet, "/v1/evidence?control="+control, nil)); list.Count != want {
				t.Errorf("filter %s: got %d, want %d", control, list.Count, want)
			}
		}
		if rr := e.as("auditor", http.MethodGet, base+"/download", nil); !bytes.Equal(rr.Body.Bytes(), content) {
			t.Errorf("download: %d", rr.Code)
		}

		for name, body := range map[string]string{
			"no title":           `{"title":"","collected_on":"2020-01-15"}`,
			"future collection":  `{"title":"x","collected_on":"2999-01-01"}`,
			"valid before taken": `{"title":"x","collected_on":"2020-01-15","valid_until":"2019-12-31"}`,
			"bad date":           `{"title":"x","collected_on":"enero"}`,
			"unknown control":    `{"title":"x","collected_on":"2020-01-15","controls":["A.1.1"]}`,
		} {
			mustStatus(t, e.as("editor", http.MethodPut, base, body), http.StatusUnprocessableEntity, name)
		}
		mustStatus(t, e.as("viewer", http.MethodPut, base, `{"title":"x"}`), http.StatusForbidden, "viewer editing")
		mustStatus(t, e.as("viewer", http.MethodPost, "/v1/evidence", formWith(nil, "x.png", []byte("x"))), http.StatusForbidden, "viewer uploading")

		mustStatus(t, e.as("editor", http.MethodDelete, base, nil), http.StatusForbidden, "editor deleting")
		mustStatus(t, e.as("admin", http.MethodDelete, base, nil), http.StatusNoContent, "delete")
		mustStatus(t, e.as("admin", http.MethodGet, base, nil), http.StatusNotFound, "get deleted")
		mustStatus(t, e.as("admin", http.MethodDelete, base, nil), http.StatusNotFound, "delete twice")
		if files := e.storedFiles(); len(files) != 0 {
			t.Errorf("deleted evidence left its file: %v", files)
		}
	})
}

func TestEvidence_IntegrityDetectsTamperingAndLoss(t *testing.T) {
	eachFileBackend(t, func(t *testing.T, e *fileEnv) {
		content := []byte("acta firmada del comité de seguridad")
		ev := decode[Evidence](t, e.as("editor", http.MethodPost, "/v1/evidence", formWith(nil, "acta.pdf", content)))
		path := "/v1/evidence/" + ev.ID + "/integrity"

		if got := decode[Integrity](t, e.as("auditor", http.MethodGet, path, nil)); !got.Intact || got.ExpectedSHA256 != hashOf(content) {
			t.Fatalf("fresh evidence should be intact: %+v", got)
		}

		// Someone alters the object directly in storage.
		stored := e.storedFiles()
		if len(stored) != 1 {
			t.Fatalf("expected one stored file, got %v", stored)
		}
		os.WriteFile(stored[0], []byte("acta modificada después de la firma"), 0o600)
		if got := decode[Integrity](t, e.as("auditor", http.MethodGet, path, nil)); got.Intact || got.ActualSHA256 == got.ExpectedSHA256 || got.Missing {
			t.Errorf("tampering not detected: %+v", got)
		}

		os.Remove(stored[0])
		if got := decode[Integrity](t, e.as("auditor", http.MethodGet, path, nil)); got.Intact || !got.Missing {
			t.Errorf("loss not detected: %+v", got)
		}
		mustStatus(t, e.as("viewer", http.MethodGet, "/v1/evidence/"+ev.ID+"/download", nil), http.StatusNotFound, "download of a lost file")
	})
}

// ── Isolation between organizations ──

func TestFiles_CrossTenantAccessIsDenied(t *testing.T) {
	eachFileBackend(t, func(t *testing.T, e *fileEnv) {
		doc := e.newDocument("Política confidencial")
		e.as("editor", http.MethodPost, "/v1/documents/"+doc.ID+"/versions/1/submit", nil)
		ev := decode[Evidence](t, e.as("editor", http.MethodPost, "/v1/evidence", formWith(nil, "secreto.txt", []byte("secreto"))))

		d, v := "/v1/documents/"+doc.ID, "/v1/evidence/"+ev.ID
		attempts := []struct {
			method, path string
			body         any
		}{
			{http.MethodGet, d, nil}, {http.MethodGet, d + "/versions/1/download", nil},
			{http.MethodGet, d + "/versions/1/integrity", nil},
			{http.MethodPut, d, `{"title":"hijacked","doc_type":"policy"}`},
			{http.MethodPost, d + "/versions", formWith(nil, "x.pdf", []byte("hijacked"))},
			{http.MethodPost, d + "/versions/1/approve", nil}, {http.MethodPost, d + "/versions/1/reject", `{"note":"x"}`},
			{http.MethodPost, d + "/obsolete", nil}, {http.MethodDelete, d, nil},
			{http.MethodGet, v, nil}, {http.MethodGet, v + "/download", nil}, {http.MethodGet, v + "/integrity", nil},
			{http.MethodPut, v, `{"title":"hijacked","collected_on":"2020-01-01"}`}, {http.MethodDelete, v, nil},
		}
		for _, a := range attempts {
			if rr := e.as("outsider", a.method, a.path, a.body); rr.Code != http.StatusNotFound {
				t.Errorf("%s %s from another organization: expected 404, got %d", a.method, a.path, rr.Code)
			}
		}
		for _, list := range []string{"/v1/documents", "/v1/evidence", "/v1/documents?control=A.5.1"} {
			if got := decode[struct{ Count int }](t, e.as("outsider", http.MethodGet, list, nil)); got.Count != 0 {
				t.Errorf("GET %s from another organization: %d records", list, got.Count)
			}
		}

		// Untouched for their owner, and nothing extra in storage.
		got := decode[Document](t, e.as("owner", http.MethodGet, d, nil))
		if got.Title != "Política confidencial" || statuses(got) != "v1=in_review" {
			t.Errorf("document was altered: %s %s", got.Title, statuses(got))
		}
		if rr := e.as("owner", http.MethodGet, v+"/download", nil); rr.Body.String() != "secreto" {
			t.Errorf("evidence was altered: %d %q", rr.Code, rr.Body.String())
		}
		if n := len(e.storedFiles()); n != 2 {
			t.Errorf("expected 2 stored files, got %d", n)
		}
	})
}

// ── PostgreSQL only ──

func TestPg_StoredFilesAreImmutableInTheDatabase(t *testing.T) {
	e := newFileEnv(t, func() *dbtest.DBs { d := dbtest.New(t); return &d }())
	doc := e.newDocument("Política")
	ev := decode[Evidence](t, e.as("editor", http.MethodPost, "/v1/evidence", formWith(nil, "a.txt", []byte("a"))))

	// Even the administrator role cannot swap the file behind a record.
	for _, stmt := range []string{
		`UPDATE document_versions SET sha256 = repeat('0', 64) WHERE document_id = '` + doc.ID + `'`,
		`UPDATE document_versions SET storage_key = 'other/key' WHERE document_id = '` + doc.ID + `'`,
		`UPDATE document_versions SET file_size = 1 WHERE document_id = '` + doc.ID + `'`,
		`UPDATE evidences SET sha256 = repeat('0', 64) WHERE id = '` + ev.ID + `'`,
		`UPDATE evidences SET file_name = 'otro.txt' WHERE id = '` + ev.ID + `'`,
	} {
		if _, err := e.dbs.Admin.Exec(stmt); err == nil || !strings.Contains(err.Error(), "immutable") {
			t.Errorf("%s: expected the immutability trigger to refuse, got %v", stmt, err)
		}
	}
	// A second approved version for the same document is impossible.
	e.dbs.Admin.Exec(`UPDATE document_versions SET status = 'approved' WHERE document_id = $1`, doc.ID)
	_, err := e.dbs.Admin.Exec(`
		INSERT INTO document_versions (tenant_id, document_id, number, status, file_name, file_size, sha256, storage_key)
		VALUES ($1, $2, 2, 'approved', 'x', 1, repeat('1', 64), 'k2')`, pgTenantA, doc.ID)
	if err == nil {
		t.Error("two approved versions of one document were accepted")
	}
}

func TestPg_FileTablesAreIsolatedByRLS(t *testing.T) {
	e := newFileEnv(t, func() *dbtest.DBs { d := dbtest.New(t); return &d }())
	e.newDocument("Política")
	e.as("editor", http.MethodPost, "/v1/evidence", formWith(map[string]string{"controls": "A.5.1"}, "a.txt", []byte("a")))

	count := func(tenantID, table string) (n int) {
		t.Helper()
		tx, _ := e.dbs.App.Begin()
		defer tx.Rollback()
		tx.Exec(`SELECT set_config('app.tenant_id', $1, true)`, tenantID)
		if err := tx.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	for _, table := range []string{"documents", "document_versions", "document_controls", "evidences", "evidence_controls"} {
		if n := count(pgTenantA, table); n == 0 {
			t.Errorf("%s: the owner sees no rows", table)
		}
		if n := count(pgTenantB, table); n != 0 {
			t.Errorf("%s: another organization sees %d rows", table, n)
		}
	}
}

func TestPg_DocumentAuditTrail(t *testing.T) {
	e := newFileEnv(t, func() *dbtest.DBs { d := dbtest.New(t); return &d }())
	doc := e.newDocument("Política")
	base := "/v1/documents/" + doc.ID
	e.as("editor", http.MethodPost, base+"/versions/1/submit", nil)
	e.as("admin", http.MethodPost, base+"/versions/1/approve", nil)
	e.as("editor", http.MethodPost, base+"/versions", formWith(nil, "v2.pdf", []byte("v2")))
	e.as("editor", http.MethodPost, base+"/versions/2/submit", nil)
	e.as("owner", http.MethodPost, base+"/versions/2/reject", `{"note":"incompleta"}`)
	e.as("owner", http.MethodPost, base+"/obsolete", nil)
	// Refused actions leave no record.
	e.as("editor", http.MethodPost, base+"/versions/2/submit", nil)
	e.as("viewer", http.MethodPost, base+"/versions", formWith(nil, "x.pdf", []byte("x")))
	ev := decode[Evidence](t, e.as("editor", http.MethodPost, "/v1/evidence", formWith(nil, "a.txt", []byte("a"))))
	e.as("admin", http.MethodDelete, "/v1/evidence/"+ev.ID, nil)

	rows, err := e.dbs.Admin.Query(`SELECT action, detail->>'actor', COALESCE(detail->>'sha256', '') FROM audit_log ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	var deletedHash string
	for rows.Next() {
		var action, actor, hash string
		rows.Scan(&action, &actor, &hash)
		got = append(got, action+" by "+strings.TrimSuffix(actor, "@empresa.test"))
		if action == "evidence.delete" {
			deletedHash = hash
		}
	}
	want := []string{
		"document.create by editor", "document.submit by editor", "document.approve by admin",
		"document.version_add by editor", "document.submit by editor", "document.reject by owner",
		"document.obsolete by owner", "evidence.create by editor", "evidence.delete by admin",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("audit trail:\n got: %v\nwant: %v", got, want)
	}
	// The hash of a deleted file stays on record.
	if deletedHash != hashOf([]byte("a")) {
		t.Errorf("evidence.delete should record the file hash, got %q", deletedHash)
	}
}

func TestPg_ConcurrentDocumentChanges(t *testing.T) {
	e := newFileEnv(t, func() *dbtest.DBs { d := dbtest.New(t); return &d }())
	doc := e.newDocument("Política")
	base := "/v1/documents/" + doc.ID

	// Ten uploads at once: ten distinct, consecutive version numbers.
	var wg sync.WaitGroup
	codes := make([]int, 10)
	for i := range codes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i] = e.as("editor", http.MethodPost, base+"/versions", formWith(nil, "v.pdf", []byte(fmt.Sprint("contenido ", i)))).Code
		}()
	}
	wg.Wait()
	for i, code := range codes {
		if code != http.StatusCreated {
			t.Errorf("upload %d: %d", i, code)
		}
	}
	got := decode[Document](t, e.as("viewer", http.MethodGet, base, nil))
	if len(got.Versions) != 11 || got.Versions[0].Number != 11 || got.Versions[10].Number != 1 {
		t.Fatalf("expected versions 11..1, got %s", statuses(got))
	}
	pending := 0
	for _, v := range got.Versions {
		if v.Status == VersionDraft {
			pending++
		}
	}
	if pending != 1 || got.Versions[0].Status != VersionDraft {
		t.Errorf("only the newest version should remain a draft: %s", statuses(got))
	}

	// Two reviewers decide at the same moment: exactly one decision stands.
	e.as("editor", http.MethodPost, base+"/versions/11/submit", nil)
	results := make([]int, 2)
	for i, action := range []string{"approve", "reject"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = e.as([]string{"owner", "admin"}[i], http.MethodPost, base+"/versions/11/"+action, `{"note":"decisión"}`).Code
		}()
	}
	wg.Wait()
	if !(results[0] == 200 && results[1] == 409) && !(results[0] == 409 && results[1] == 200) {
		t.Errorf("expected one decision to win and the other to conflict, got %v", results)
	}
}

func hashOf(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func TestJSONShape_StorageKeyNeverLeaves(t *testing.T) {
	e := newFileEnv(t, nil)
	doc := e.newDocument("Política")
	ev := e.as("editor", http.MethodPost, "/v1/evidence", formWith(nil, "a.txt", []byte("a")))
	for name, body := range map[string]string{
		"document": e.as("viewer", http.MethodGet, "/v1/documents/"+doc.ID, nil).Body.String(),
		"list":     e.as("viewer", http.MethodGet, "/v1/documents", nil).Body.String(),
		"evidence": ev.Body.String(),
	} {
		if strings.Contains(body, "/documents/") || strings.Contains(body, "/evidence/") || strings.Contains(strings.ToLower(body), "storage") {
			t.Errorf("%s response exposes the storage key: %s", name, body)
		}
		var generic map[string]any
		if err := json.Unmarshal([]byte(body), &generic); err != nil {
			t.Errorf("%s: invalid JSON: %v", name, err)
		}
	}
}
