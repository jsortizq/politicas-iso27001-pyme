package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Dir stores objects as files under a root directory.
type Dir struct {
	root string
}

// NewDir creates the root directory if needed and returns the store.
func NewDir(root string) (*Dir, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("storage: resolve %q: %w", root, err)
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, fmt.Errorf("storage: create %q: %w", abs, err)
	}
	return &Dir{root: abs}, nil
}

func (d *Dir) path(key string) (string, error) {
	if !ValidKey(key) {
		return "", errInvalidKey
	}
	return filepath.Join(d.root, filepath.FromSlash(key)), nil
}

func (d *Dir) Put(_ context.Context, key string, r io.Reader, size int64, sha256Hex string) error {
	path, err := d.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("storage: put %s: %w", key, err)
	}

	// Write to a temporary file and rename, so a failed upload never leaves
	// a partial object under the final name.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".upload-*")
	if err != nil {
		return fmt.Errorf("storage: put %s: %w", key, err)
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // no-op after rename

	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(r, size+1))
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("storage: put %s: %w", key, err)
	}
	if written != size {
		return fmt.Errorf("storage: put %s: expected %d bytes, got %d", key, size, written)
	}
	if got := hex.EncodeToString(hash.Sum(nil)); got != sha256Hex {
		return fmt.Errorf("storage: put %s: content does not match its SHA-256", key)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("storage: put %s: %w", key, err)
	}
	return nil
}

func (d *Dir) Get(_ context.Context, key string) (io.ReadCloser, error) {
	path, err := d.path(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("storage: get %s: %w", key, err)
	}
	return f, nil
}

func (d *Dir) Delete(_ context.Context, key string) error {
	path, err := d.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("storage: delete %s: %w", key, err)
	}
	return nil
}
