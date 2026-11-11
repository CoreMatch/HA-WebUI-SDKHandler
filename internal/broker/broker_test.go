package broker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// gzipMagicBody returns a payload whose leading bytes satisfy the gzip magic
// check in archiveSuffix, so Download recognizes it as a .tar.gz archive.
func gzipMagicBody(payload string) []byte {
	return append([]byte{0x1f, 0x8b, 0x08, 0x00}, []byte(payload)...)
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// newDownloadTestServer emulates the two upstream endpoints Download touches:
// the OAuth2 token endpoint and the package download endpoint (serving body).
func newDownloadTestServer(body []byte) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/oauth/token":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data":    map[string]any{"access_token": "test-token", "expires_in": 3600},
			})
		case strings.HasPrefix(r.URL.Path, "/services/sdk-packages/") && strings.HasSuffix(r.URL.Path, "/download"):
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
}

func newTestClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	c, err := New(baseURL, "client-id", "client-secret", t.TempDir(), 5*time.Second)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestVerifySHA256(t *testing.T) {
	sum := sha256Hex([]byte("payload"))
	cases := []struct {
		name     string
		expected string
		actual   string
		wantErr  bool
	}{
		{"exact match", sum, sum, false},
		{"case-insensitive match", strings.ToUpper(sum), sum, false},
		{"surrounding whitespace", "  " + sum + "\n", sum, false},
		{"mismatch", sha256Hex([]byte("other")), sum, true},
		{"empty expected", "", sum, true},
		{"whitespace-only expected", "   ", sum, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := verifySHA256("pkg", tc.expected, tc.actual)
			if (err != nil) != tc.wantErr {
				t.Fatalf("verifySHA256 err = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

func TestDownload_SHA256Match(t *testing.T) {
	body := gzipMagicBody("sdk-archive-payload")
	sum := sha256Hex(body)
	srv := newDownloadTestServer(body)
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	dest, got, err := c.Download("mypkg", sum)
	if err != nil {
		t.Fatalf("Download returned error on matching hash: %v", err)
	}
	if got != sum {
		t.Fatalf("returned sum = %s, want %s", got, sum)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("downloaded file missing: %v", err)
	}
	if !strings.HasSuffix(dest, "mypkg.tar.gz") {
		t.Fatalf("unexpected dest name: %s", dest)
	}
}

func TestDownload_SHA256MismatchFails(t *testing.T) {
	body := gzipMagicBody("sdk-archive-payload")
	srv := newDownloadTestServer(body)
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	wrong := sha256Hex([]byte("tampered"))
	dest, _, err := c.Download("mypkg", wrong)
	if err == nil {
		t.Fatalf("Download succeeded despite sha256 mismatch; want error")
	}
	if !strings.Contains(err.Error(), "sha256 不匹配") {
		t.Fatalf("error = %q, want a sha256 mismatch message", err.Error())
	}
	if dest != "" {
		t.Fatalf("dest = %q on failure, want empty", dest)
	}
	// The corrupt cache file must be removed so it cannot be consumed later.
	if _, statErr := os.Stat(filepath.Join(c.cacheDir, "mypkg.tar.gz")); !os.IsNotExist(statErr) {
		t.Fatalf("corrupt cache file was not removed (stat err = %v)", statErr)
	}
}

func TestDownload_SHA256MissingFails(t *testing.T) {
	body := gzipMagicBody("sdk-archive-payload")
	srv := newDownloadTestServer(body)
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	dest, _, err := c.Download("mypkg", "")
	if err == nil {
		t.Fatalf("Download succeeded despite missing sha256; want error")
	}
	if !strings.Contains(err.Error(), "缺少 sha256") {
		t.Fatalf("error = %q, want a missing-sha256 message", err.Error())
	}
	if dest != "" {
		t.Fatalf("dest = %q on failure, want empty", dest)
	}
	if _, statErr := os.Stat(filepath.Join(c.cacheDir, "mypkg.tar.gz")); !os.IsNotExist(statErr) {
		t.Fatalf("cache file was not removed on missing hash (stat err = %v)", statErr)
	}
}
