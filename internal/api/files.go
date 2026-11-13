package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/jsortizq/politicas-iso27001-pyme/internal/storage"
)

// File describes a stored file. The storage key never leaves the server.
type File struct {
	Name       string `json:"file_name"`
	MimeType   string `json:"mime_type"`
	Size       int64  `json:"file_size"`
	SHA256     string `json:"sha256"`
	StorageKey string `json:"-"`
}

// Files handles uploads and downloads for documents and evidence.
//
// Uploads are buffered to a temporary file while their SHA-256 is computed,
// so the size limit is enforced and the hash is known before anything
// reaches object storage. Downloads are always sent as attachments with a
// generic content type: an uploaded file is never rendered by the browser
// in the application's origin, whatever it contains.
type Files struct {
	store    storage.Store
	maxBytes int64
	logger   *slog.Logger
}

// NewFiles creates the helper. maxBytes caps the size of one uploaded file.
func NewFiles(store storage.Store, maxBytes int64, logger *slog.Logger) *Files {
	return &Files{store: store, maxBytes: maxBytes, logger: logger}
}

// uploadError is a client error raised while receiving an upload.
type uploadError struct {
	status int
	msg    string
}

func (e *uploadError) Error() string { return e.msg }

// writeUploadError reports err to the client; unexpected errors are logged.
func (f *Files) writeUploadError(w http.ResponseWriter, err error) {
	var ue *uploadError
	if errors.As(err, &ue) {
		writeError(w, ue.status, ue.msg)
		return
	}
	f.logger.Error("upload failed", "error", err)
	writeError(w, http.StatusInternalServerError, "failed to store the file")
}

// upload is a received file waiting in a temporary file.
type upload struct {
	File
	tmp *os.File
}

// discard removes the temporary file. Safe to call on a nil upload.
func (u *upload) discard() {
	if u == nil || u.tmp == nil {
		return
	}
	u.tmp.Close()
	os.Remove(u.tmp.Name())
}

const (
	maxFormFields    = 50
	maxFormFieldSize = 64 << 10
	transferDeadline = 10 * time.Minute
)

// receive reads a multipart/form-data request: text fields plus at most one
// file in the "file" part. The caller must discard the returned upload.
// When requireFile is true a request without a file is rejected.
func (f *Files) receive(w http.ResponseWriter, r *http.Request, requireFile bool) (url.Values, *upload, error) {
	// Large files need more time than the server's default deadlines.
	http.NewResponseController(w).SetReadDeadline(time.Now().Add(transferDeadline)) //nolint:errcheck // not all writers support it

	r.Body = http.MaxBytesReader(w, r.Body, f.maxBytes+maxFormFields*maxFormFieldSize)
	reader, err := r.MultipartReader()
	if err != nil {
		return nil, nil, &uploadError{http.StatusBadRequest, "expected a multipart/form-data request"}
	}

	fields := url.Values{}
	var up *upload
	fail := func(err error) (url.Values, *upload, error) {
		up.discard()
		return nil, nil, err
	}

	for parts := 0; ; parts++ {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				return fail(f.tooLarge())
			}
			return fail(&uploadError{http.StatusBadRequest, "malformed multipart request"})
		}
		if parts >= maxFormFields {
			return fail(&uploadError{http.StatusBadRequest, "too many form fields"})
		}

		if part.FormName() != "file" || part.FileName() == "" {
			value, err := io.ReadAll(io.LimitReader(part, maxFormFieldSize+1))
			if err != nil || len(value) > maxFormFieldSize {
				return fail(&uploadError{http.StatusBadRequest, "form field too large"})
			}
			fields.Add(part.FormName(), string(value))
			continue
		}
		if up != nil {
			return fail(&uploadError{http.StatusBadRequest, "only one file per request"})
		}

		tmp, err := os.CreateTemp("", "sgsi-upload-*")
		if err != nil {
			return fail(fmt.Errorf("create temporary file: %w", err))
		}
		up = &upload{tmp: tmp}
		hash := sha256.New()
		size, err := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(part, f.maxBytes+1))
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				return fail(f.tooLarge())
			}
			return fail(&uploadError{http.StatusBadRequest, "the upload was interrupted"})
		}
		if size > f.maxBytes {
			return fail(f.tooLarge())
		}
		if size == 0 {
			return fail(&uploadError{http.StatusUnprocessableEntity, "the file is empty"})
		}
		name := cleanFileName(part.FileName())
		up.File = File{
			Name:     name,
			MimeType: mimeTypeOf(name),
			Size:     size,
			SHA256:   hex.EncodeToString(hash.Sum(nil)),
		}
	}

	if up == nil && requireFile {
		return nil, nil, &uploadError{http.StatusUnprocessableEntity, "a file is required"}
	}
	return fields, up, nil
}

func (f *Files) tooLarge() error {
	return &uploadError{http.StatusRequestEntityTooLarge,
		fmt.Sprintf("the file exceeds the %d MB limit", f.maxBytes>>20)}
}

// save copies a received upload to object storage under key.
func (f *Files) save(ctx context.Context, key string, up *upload) error {
	if _, err := up.tmp.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err := f.store.Put(ctx, key, up.tmp, up.Size, up.SHA256); err != nil {
		return err
	}
	up.StorageKey = key
	return nil
}

// remove deletes stored objects, logging (not failing) on error: the
// database is the source of truth and an orphaned object is harmless.
func (f *Files) remove(ctx context.Context, keys ...string) {
	for _, key := range keys {
		if key == "" {
			continue
		}
		if err := f.store.Delete(ctx, key); err != nil {
			f.logger.Warn("could not delete stored object", "key", key, "error", err)
		}
	}
}

// serve streams a stored file as a download.
func (f *Files) serve(w http.ResponseWriter, r *http.Request, file File) {
	body, err := f.store.Get(r.Context(), file.StorageKey)
	if errors.Is(err, storage.ErrNotFound) {
		f.logger.Error("stored object is missing", "key", file.StorageKey)
		writeError(w, http.StatusNotFound, "the file is missing from storage")
		return
	}
	if err != nil {
		f.logger.Error("download failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to read the file")
		return
	}
	defer body.Close()

	http.NewResponseController(w).SetWriteDeadline(time.Now().Add(transferDeadline)) //nolint:errcheck

	h := w.Header()
	// Never let the browser interpret uploaded content in this origin.
	h.Set("Content-Type", "application/octet-stream")
	h.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": file.Name}))
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	h.Set("Cache-Control", "private, no-store")
	h.Set("Content-Length", fmt.Sprint(file.Size))
	io.Copy(w, body) //nolint:errcheck // client went away
}

// Integrity is the result of re-reading a stored file and hashing it.
type Integrity struct {
	Intact         bool   `json:"intact"`
	ExpectedSHA256 string `json:"expected_sha256"`
	ActualSHA256   string `json:"actual_sha256,omitempty"`
	Missing        bool   `json:"missing,omitempty"`
}

// verify recomputes the SHA-256 of the stored object and compares it with
// the hash recorded when the file was uploaded.
func (f *Files) verify(ctx context.Context, file File) (Integrity, error) {
	result := Integrity{ExpectedSHA256: file.SHA256}
	body, err := f.store.Get(ctx, file.StorageKey)
	if errors.Is(err, storage.ErrNotFound) {
		result.Missing = true
		return result, nil
	}
	if err != nil {
		return result, err
	}
	defer body.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, body); err != nil {
		return result, err
	}
	result.ActualSHA256 = hex.EncodeToString(hash.Sum(nil))
	result.Intact = result.ActualSHA256 == file.SHA256
	return result, nil
}

func (f *Files) writeIntegrity(w http.ResponseWriter, r *http.Request, file File) {
	result, err := f.verify(r.Context(), file)
	if err != nil {
		f.logger.Error("integrity check failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to read the file")
		return
	}
	if !result.Intact {
		f.logger.Error("stored file failed its integrity check", "key", file.StorageKey, "missing", result.Missing)
	}
	writeJSON(w, http.StatusOK, result)
}

// cleanFileName keeps the base name of what the client sent, without
// control characters, so it is safe to show and to use in a download header.
func cleanFileName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(filepath.FromSlash(name))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '/' || r == '\\' {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if runes := []rune(name); len(runes) > 200 {
		ext := filepath.Ext(name)
		if len([]rune(ext)) > 20 {
			ext = ""
		}
		name = string(runes[:200-len([]rune(ext))]) + ext
	}
	if name == "" || name == "." || name == ".." {
		return "archivo"
	}
	return name
}

// mimeTypeOf guesses a type from the file extension. It is informational:
// downloads are always served as application/octet-stream.
func mimeTypeOf(name string) string {
	if t := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); t != "" {
		mediaType, _, err := mime.ParseMediaType(t)
		if err == nil {
			return mediaType
		}
	}
	return "application/octet-stream"
}

// ── Shared validation for records linked to controls ──

// parseDate validates an optional YYYY-MM-DD date.
func parseDate(field, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if _, err := time.Parse("2006-01-02", value); err != nil {
		return "", invalid("%s must be YYYY-MM-DD", field)
	}
	return value, nil
}

// today returns the current date as YYYY-MM-DD (UTC).
func today() string { return time.Now().UTC().Format("2006-01-02") }
