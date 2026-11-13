package storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var ctx = context.Background()

func sum(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func TestValidKey(t *testing.T) {
	valid := []string{
		"a", "tenant/evidence/123e4567-e89b-42d3-a456-426614174000",
		"t/documents/doc/v1-file.pdf", "A_b-c.d/e",
	}
	invalid := []string{
		"", "/abs", "a//b", "a/", "../etc/passwd", "a/../b", "a/./b", ".hidden", "a/.hidden",
		"a b", "a\\b", "a\x00b", "ñ", "a?b", "a#b", "a%2e%2e", strings.Repeat("a", 513),
	}
	for _, k := range valid {
		if !ValidKey(k) {
			t.Errorf("ValidKey(%q) = false, want true", k)
		}
	}
	for _, k := range invalid {
		if ValidKey(k) {
			t.Errorf("ValidKey(%q) = true, want false", k)
		}
	}
}

// contract is the behaviour every backend must have.
func contract(t *testing.T, store Store) {
	t.Helper()
	data := make([]byte, 300_000)
	rand.Read(data)
	key := "tenant-a/evidence/" + hex.EncodeToString(data[:8])

	if _, err := store.Get(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get before Put: expected ErrNotFound, got %v", err)
	}
	if err := store.Put(ctx, key, bytes.NewReader(data), int64(len(data)), sum(data)); err != nil {
		t.Fatalf("Put: %v", err)
	}
	r, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got, _ := io.ReadAll(r)
	r.Close()
	if !bytes.Equal(got, data) {
		t.Fatalf("content differs: got %d bytes, want %d", len(got), len(data))
	}

	// Empty objects are legal.
	if err := store.Put(ctx, key+"-empty", bytes.NewReader(nil), 0, sum(nil)); err != nil {
		t.Errorf("Put empty: %v", err)
	}

	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after Delete: expected ErrNotFound, got %v", err)
	}
	if err := store.Delete(ctx, key); err != nil {
		t.Errorf("deleting a missing object should not fail: %v", err)
	}
	store.Delete(ctx, key+"-empty")

	for _, bad := range []string{"../escape", "a//b", ""} {
		if err := store.Put(ctx, bad, strings.NewReader("x"), 1, sum([]byte("x"))); err == nil {
			t.Errorf("Put(%q) accepted an invalid key", bad)
		}
		if _, err := store.Get(ctx, bad); err == nil {
			t.Errorf("Get(%q) accepted an invalid key", bad)
		}
		if err := store.Delete(ctx, bad); err == nil {
			t.Errorf("Delete(%q) accepted an invalid key", bad)
		}
	}
}

// ── Dir ──

func TestDir_Contract(t *testing.T) {
	store, err := NewDir(filepath.Join(t.TempDir(), "nested", "root"))
	if err != nil {
		t.Fatal(err)
	}
	contract(t, store)
}

func TestDir_RejectsCorruptOrTruncatedUploads(t *testing.T) {
	root := t.TempDir()
	store, _ := NewDir(root)
	data := []byte("contenido de la evidencia")

	cases := map[string]struct {
		body []byte
		size int64
		hash string
	}{
		"wrong hash":            {data, int64(len(data)), sum([]byte("otra cosa"))},
		"shorter than declared": {data[:5], int64(len(data)), sum(data)},
		"longer than declared":  {data, 5, sum(data[:5])},
	}
	for name, c := range cases {
		if err := store.Put(ctx, "t/evidence/x", bytes.NewReader(c.body), c.size, c.hash); err == nil {
			t.Errorf("%s: Put succeeded", name)
		}
		if _, err := store.Get(ctx, "t/evidence/x"); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: a rejected upload left an object behind (%v)", name, err)
		}
	}
	// No temporary files are left either.
	entries, _ := os.ReadDir(filepath.Join(root, "t", "evidence"))
	if len(entries) != 0 {
		t.Errorf("leftover files after rejected uploads: %v", entries)
	}
}

// ── Signature Version 4 ──

// The examples published in the Amazon S3 API reference ("Signature
// Calculations for the Authorization Header") give the exact signature
// expected for these requests.
func TestSignV4_AWSPublishedExamples(t *testing.T) {
	const (
		accessKey = "AKIAIOSFODNN7EXAMPLE"
		secretKey = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	)
	at := time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name, method, url string
		headers           map[string]string
		payload           string
		wantSigned        string
		wantSignature     string
	}{
		{
			name: "GET object", method: "GET", url: "https://examplebucket.s3.amazonaws.com/test.txt",
			headers:       map[string]string{"Range": "bytes=0-9"},
			wantSigned:    "host;range;x-amz-content-sha256;x-amz-date",
			wantSignature: "f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41",
		},
		{
			name: "PUT object", method: "PUT", url: "https://examplebucket.s3.amazonaws.com/test$file.text",
			headers: map[string]string{
				"Date":                "Fri, 24 May 2013 00:00:00 GMT",
				"X-Amz-Storage-Class": "REDUCED_REDUNDANCY",
			},
			payload:       "Welcome to Amazon S3.",
			wantSigned:    "date;host;x-amz-content-sha256;x-amz-date;x-amz-storage-class",
			wantSignature: "98ad721746da40c64f1a55b78f14c238d841ea1380cd77a1b5971af0ece108bd",
		},
		{
			name: "GET bucket lifecycle", method: "GET", url: "https://examplebucket.s3.amazonaws.com/?lifecycle",
			wantSigned:    "host;x-amz-content-sha256;x-amz-date",
			wantSignature: "fea454ca298b7da1c68078a5d1bdbfbbe0d65c699e0f91ac7a200a0136783543",
		},
		{
			name: "list objects", method: "GET", url: "https://examplebucket.s3.amazonaws.com/?max-keys=2&prefix=J",
			wantSigned:    "host;x-amz-content-sha256;x-amz-date",
			wantSignature: "34b48302e7b5fa45bde8084f4b7868a86f0a534bc59db6670ed5711ef69dc6f7",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req, err := http.NewRequest(c.method, c.url, nil)
			if err != nil {
				t.Fatal(err)
			}
			for k, v := range c.headers {
				req.Header.Set(k, v)
			}
			signV4(req, accessKey, secretKey, "us-east-1", "s3", sum([]byte(c.payload)), at)

			want := "AWS4-HMAC-SHA256 Credential=" + accessKey + "/20130524/us-east-1/s3/aws4_request," +
				"SignedHeaders=" + c.wantSigned + ",Signature=" + c.wantSignature
			if got := req.Header.Get("Authorization"); got != want {
				t.Errorf("Authorization\n got: %s\nwant: %s", got, want)
			}
		})
	}
}

func TestAWSEscape(t *testing.T) {
	for in, want := range map[string]string{
		"abc-_.~XYZ019": "abc-_.~XYZ019",
		"a b":           "a%20b",
		"a/b":           "a%2Fb",
		"test$file":     "test%24file",
		"ñ":             "%C3%B1",
		"a+b=c&d":       "a%2Bb%3Dc%26d",
	} {
		if got := awsEscape(in); got != want {
			t.Errorf("awsEscape(%q) = %q, want %q", in, got, want)
		}
	}
}

// ── S3 against a local stand-in ──

// fakeS3 is a tiny S3-compatible server. It checks every request the way a
// real service does: the signature must be valid for the request as it
// arrived on the wire, and the body must match the declared SHA-256.
type fakeS3 struct {
	t         *testing.T
	access    string
	secret    string
	region    string
	mu        sync.Mutex
	buckets   map[string]bool
	objects   map[string][]byte
	clockSkew time.Duration
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)

	// Re-sign what arrived and compare with what the client sent.
	sent := r.Header.Get("Authorization")
	at, err := time.Parse("20060102T150405Z", r.Header.Get("X-Amz-Date"))
	if err != nil || sent == "" {
		http.Error(w, "AccessDenied", http.StatusForbidden)
		return
	}
	signedNames := strings.Split(strings.TrimPrefix(sent[strings.Index(sent, "SignedHeaders=")+len("SignedHeaders="):strings.Index(sent, ",Signature=")], ""), ";")
	check, _ := http.NewRequest(r.Method, "http://"+r.Host+r.URL.RequestURI(), nil)
	for _, name := range signedNames {
		if name != "host" {
			check.Header.Set(name, r.Header.Get(name))
		}
	}
	signV4(check, f.access, f.secret, f.region, "s3", r.Header.Get("X-Amz-Content-Sha256"), at)
	if check.Header.Get("Authorization") != sent {
		http.Error(w, "SignatureDoesNotMatch", http.StatusForbidden)
		return
	}
	if sum(body) != r.Header.Get("X-Amz-Content-Sha256") {
		http.Error(w, "XAmzContentSHA256Mismatch", http.StatusBadRequest)
		return
	}

	bucket, key, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	f.mu.Lock()
	defer f.mu.Unlock()
	if key == "" { // bucket operations
		switch r.Method {
		case http.MethodHead:
			if !f.buckets[bucket] {
				w.WriteHeader(http.StatusNotFound)
			}
		case http.MethodPut:
			f.buckets[bucket] = true
		}
		return
	}
	if !f.buckets[bucket] {
		http.Error(w, "NoSuchBucket", http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodPut:
		f.objects[bucket+"/"+key] = body
	case http.MethodGet:
		data, ok := f.objects[bucket+"/"+key]
		if !ok {
			http.Error(w, "NoSuchKey", http.StatusNotFound)
			return
		}
		w.Write(data)
	case http.MethodDelete:
		delete(f.objects, bucket+"/"+key)
		w.WriteHeader(http.StatusNoContent)
	}
}

func newFakeS3(t *testing.T) (*fakeS3, S3Config) {
	fake := &fakeS3{t: t, access: "test-access", secret: "test-secret/with+symbols", region: "us-east-1",
		buckets: map[string]bool{}, objects: map[string][]byte{}}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	return fake, S3Config{Endpoint: srv.URL, Region: fake.region, Bucket: "sgsi-test",
		AccessKey: fake.access, SecretKey: fake.secret, PathStyle: true}
}

func TestS3_ContractAgainstSigningServer(t *testing.T) {
	_, cfg := newFakeS3(t)
	store, err := NewS3(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket: %v", err)
	}
	if err := store.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket on an existing bucket: %v", err)
	}
	contract(t, store)
}

func TestS3_WrongSecretIsRejected(t *testing.T) {
	_, cfg := newFakeS3(t)
	cfg.SecretKey = "not-the-secret"
	store, _ := NewS3(cfg)
	if err := store.EnsureBucket(ctx); err == nil {
		t.Error("EnsureBucket succeeded with a wrong secret")
	}
	err := store.Put(ctx, "t/evidence/x", strings.NewReader("x"), 1, sum([]byte("x")))
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Errorf("Put with a wrong secret: %v", err)
	}
}

func TestS3_CorruptedUploadIsRejectedByTheService(t *testing.T) {
	_, cfg := newFakeS3(t)
	store, _ := NewS3(cfg)
	store.EnsureBucket(ctx)
	// The declared hash does not match the bytes sent.
	if err := store.Put(ctx, "t/evidence/x", strings.NewReader("tampered"), 8, sum([]byte("original"))); err == nil {
		t.Error("Put succeeded although content and SHA-256 differ")
	}
}

func TestNewS3_Validation(t *testing.T) {
	ok := S3Config{Endpoint: "http://localhost:9000", Region: "us-east-1", Bucket: "b", AccessKey: "a", SecretKey: "s"}
	if _, err := NewS3(ok); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	for name, mutate := range map[string]func(c *S3Config){
		"no endpoint": func(c *S3Config) { c.Endpoint = "" },
		"bad scheme":  func(c *S3Config) { c.Endpoint = "ftp://host" },
		"no bucket":   func(c *S3Config) { c.Bucket = "" },
		"no region":   func(c *S3Config) { c.Region = "" },
		"no access":   func(c *S3Config) { c.AccessKey = "" },
		"no secret":   func(c *S3Config) { c.SecretKey = "" },
	} {
		c := ok
		mutate(&c)
		if _, err := NewS3(c); err == nil {
			t.Errorf("%s: config accepted", name)
		}
	}
}

// ── S3 against a real service ──

// Runs against a real S3-compatible service (MinIO in CI) when
// TEST_S3_ENDPOINT, TEST_S3_ACCESS_KEY and TEST_S3_SECRET_KEY are set.
func TestS3_ContractAgainstRealService(t *testing.T) {
	endpoint := os.Getenv("TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("TEST_S3_ENDPOINT not set; skipping test against a real S3 service")
	}
	suffix := make([]byte, 4)
	rand.Read(suffix)
	cfg := S3Config{
		Endpoint:  endpoint,
		Region:    "us-east-1",
		Bucket:    "sgsi-test-" + hex.EncodeToString(suffix),
		AccessKey: os.Getenv("TEST_S3_ACCESS_KEY"),
		SecretKey: os.Getenv("TEST_S3_SECRET_KEY"),
		PathStyle: true,
	}
	store, err := NewS3(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket: %v", err)
	}
	if err := store.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket on an existing bucket: %v", err)
	}
	contract(t, store)

	// A real service must refuse a wrong secret and a mismatching hash.
	bad := cfg
	bad.SecretKey = "wrong-secret"
	intruder, _ := NewS3(bad)
	if err := intruder.Put(ctx, "t/evidence/x", strings.NewReader("x"), 1, sum([]byte("x"))); err == nil {
		t.Error("Put succeeded with a wrong secret")
	}
	if err := store.Put(ctx, "t/evidence/y", strings.NewReader("tampered"), 8, sum([]byte("original"))); err == nil {
		t.Error("Put succeeded although content and SHA-256 differ")
	}
}
