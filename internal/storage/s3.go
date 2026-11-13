package storage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// S3Config configures an S3-compatible backend.
type S3Config struct {
	Endpoint  string // e.g. http://localhost:9000 or https://s3.us-east-1.amazonaws.com
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
	// PathStyle addresses the bucket as endpoint/bucket/key (SeaweedFS, MinIO and most
	// self-hosted services). When false the bucket goes in the host name
	// (bucket.endpoint/key), as AWS S3 prefers.
	PathStyle bool
}

// S3 stores objects in an S3-compatible service. Requests are signed with
// AWS Signature Version 4, implemented here with the standard library.
type S3 struct {
	cfg      S3Config
	endpoint *url.URL
	client   *http.Client
	now      func() time.Time
}

// emptySHA256 is the SHA-256 of an empty payload.
const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// NewS3 validates the configuration and returns the store.
func NewS3(cfg S3Config) (*S3, error) {
	endpoint, err := url.Parse(strings.TrimRight(cfg.Endpoint, "/"))
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return nil, fmt.Errorf("storage: S3 endpoint must be an http(s) URL, got %q", cfg.Endpoint)
	}
	if cfg.Bucket == "" || cfg.Region == "" || cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, fmt.Errorf("storage: S3 bucket, region, access key and secret key are required")
	}
	return &S3{
		cfg:      cfg,
		endpoint: endpoint,
		// No overall timeout: uploads and downloads of large files are
		// bounded by the request context instead.
		client: &http.Client{Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			ResponseHeaderTimeout: 30 * time.Second,
			IdleConnTimeout:       90 * time.Second,
			MaxIdleConnsPerHost:   8,
		}},
		now: time.Now,
	}, nil
}

// EnsureBucket creates the bucket if it does not exist. It is meant for
// development (the local service starts empty); in production the bucket is normally
// provisioned separately and this is a cheap existence check.
func (s *S3) EnsureBucket(ctx context.Context) error {
	resp, err := s.do(ctx, http.MethodHead, "", nil, 0, emptySHA256)
	if err != nil {
		return fmt.Errorf("storage: check bucket: %w", err)
	}
	drain(resp)
	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusNotFound:
	default:
		return fmt.Errorf("storage: check bucket %q: unexpected status %d", s.cfg.Bucket, resp.StatusCode)
	}

	resp, err = s.do(ctx, http.MethodPut, "", nil, 0, emptySHA256)
	if err != nil {
		return fmt.Errorf("storage: create bucket: %w", err)
	}
	defer drain(resp)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
		return fmt.Errorf("storage: create bucket %q: %s", s.cfg.Bucket, describe(resp))
	}
	return nil
}

func (s *S3) Put(ctx context.Context, key string, r io.Reader, size int64, sha256Hex string) error {
	if !ValidKey(key) {
		return errInvalidKey
	}
	resp, err := s.do(ctx, http.MethodPut, key, r, size, sha256Hex)
	if err != nil {
		return fmt.Errorf("storage: put %s: %w", key, err)
	}
	defer drain(resp)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("storage: put %s: %s", key, describe(resp))
	}
	return nil
}

func (s *S3) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if !ValidKey(key) {
		return nil, errInvalidKey
	}
	resp, err := s.do(ctx, http.MethodGet, key, nil, 0, emptySHA256)
	if err != nil {
		return nil, fmt.Errorf("storage: get %s: %w", key, err)
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return resp.Body, nil
	case http.StatusNotFound:
		drain(resp)
		return nil, ErrNotFound
	default:
		defer drain(resp)
		return nil, fmt.Errorf("storage: get %s: %s", key, describe(resp))
	}
}

func (s *S3) Delete(ctx context.Context, key string) error {
	if !ValidKey(key) {
		return errInvalidKey
	}
	resp, err := s.do(ctx, http.MethodDelete, key, nil, 0, emptySHA256)
	if err != nil {
		return fmt.Errorf("storage: delete %s: %w", key, err)
	}
	defer drain(resp)
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("storage: delete %s: %s", key, describe(resp))
	}
	return nil
}

// do sends a signed request for the object key ("" addresses the bucket).
func (s *S3) do(ctx context.Context, method, key string, body io.Reader, size int64, payloadSHA256 string) (*http.Response, error) {
	u := *s.endpoint
	path := "/" + key
	if s.cfg.PathStyle {
		path = "/" + s.cfg.Bucket + path
	} else {
		u.Host = s.cfg.Bucket + "." + u.Host
	}
	if key == "" {
		path = strings.TrimSuffix(path, "/")
		if path == "" {
			path = "/"
		}
	}
	u.Path = s.endpoint.Path + path

	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.ContentLength = size
	}
	signV4(req, s.cfg.AccessKey, s.cfg.SecretKey, s.cfg.Region, "s3", payloadSHA256, s.now())
	return s.client.Do(req)
}

// ── AWS Signature Version 4 ──

// signV4 signs req in place. It signs the Host header, every x-amz-*
// header and any other header already set on the request.
func signV4(req *http.Request, accessKey, secretKey, region, service, payloadSHA256 string, now time.Time) {
	now = now.UTC()
	amzDate := now.Format("20060102T150405Z")
	date := now.Format("20060102")

	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadSHA256)

	// Canonical headers: lower-case names, trimmed values, sorted by name.
	headers := map[string]string{"host": req.URL.Host}
	for name, values := range req.Header {
		headers[strings.ToLower(name)] = strings.Join(strings.Fields(strings.Join(values, ",")), " ")
	}
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	var canonicalHeaders strings.Builder
	for _, name := range names {
		canonicalHeaders.WriteString(name + ":" + headers[name] + "\n")
	}
	signedHeaders := strings.Join(names, ";")

	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI(req.URL.Path),
		canonicalQuery(req.URL.Query()),
		canonicalHeaders.String(),
		signedHeaders,
		payloadSHA256,
	}, "\n")

	scope := date + "/" + region + "/" + service + "/aws4_request"
	requestHash := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(requestHash[:])

	key := hmacSHA256([]byte("AWS4"+secretKey), date)
	key = hmacSHA256(key, region)
	key = hmacSHA256(key, service)
	key = hmacSHA256(key, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(key, stringToSign))

	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s,SignedHeaders=%s,Signature=%s",
		accessKey, scope, signedHeaders, signature))
}

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

// canonicalURI percent-encodes each path segment as S3 expects (once, with
// "/" kept as the separator).
func canonicalURI(path string) string {
	if path == "" {
		return "/"
	}
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		segments[i] = awsEscape(segment)
	}
	return strings.Join(segments, "/")
}

func canonicalQuery(values url.Values) string {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		vs := append([]string(nil), values[k]...)
		sort.Strings(vs)
		for _, v := range vs {
			parts = append(parts, awsEscape(k)+"="+awsEscape(v))
		}
	}
	return strings.Join(parts, "&")
}

// awsEscape percent-encodes everything except the RFC 3986 unreserved
// characters, with upper-case hex digits, as Signature Version 4 requires.
func awsEscape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' ||
			c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func drain(resp *http.Response) {
	io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10)) //nolint:errcheck
	resp.Body.Close()
}

// describe summarises an error response without dumping its whole body.
func describe(resp *http.Response) string {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return fmt.Sprintf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}
