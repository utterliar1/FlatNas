package handlers

import (
	"net/http"
	"testing"
	"time"
)

type redirectRoundTripper struct{}

func (redirectRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusFound,
		Status:     "302 Found",
		Header:     make(http.Header),
		Body:       http.NoBody,
		Request:    req,
	}, nil
}

func TestValidateExternalHTTPURLRejectsUnsafeTargets(t *testing.T) {
	for _, raw := range []string{
		"file:///etc/passwd",
		"http://user:pass@example.com/feed",
		"http://127.0.0.1/feed",
		"http://localhost/feed",
	} {
		if _, err := validateExternalHTTPURL(raw); err == nil {
			t.Fatalf("expected URL to be rejected: %s", raw)
		}
	}
}

func TestProxyResponseHeadersDropSensitiveAndHopByHopHeaders(t *testing.T) {
	for _, header := range []string{"Set-Cookie", "Authorization", "Connection", "Transfer-Encoding", "Upgrade"} {
		if !shouldDropProxyResponseHeader(header) {
			t.Fatalf("header %q should be dropped", header)
		}
	}
	if shouldDropProxyResponseHeader("Content-Type") {
		t.Fatal("content type should remain available")
	}
}

func TestProxyClientDoesNotFollowRedirects(t *testing.T) {
	client := cloneProxyClientWithoutRedirects(&http.Client{Transport: redirectRoundTripper{}})
	response, err := client.Get("https://example.com")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusFound {
		t.Fatalf("expected redirect response, got %d", response.StatusCode)
	}
}

func TestSafeHTTPClientDoesNotUseEnvironmentProxy(t *testing.T) {
	client := newSafeHTTPClient(time.Second)
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected safe HTTP transport")
	}
	if transport.Proxy != nil {
		t.Fatal("safe HTTP client must not delegate target resolution to an external proxy")
	}
}

func TestValidUploadSessionRejectsInvalidSizeAndFutureTimestamp(t *testing.T) {
	base := UploadSession{
		Size:        32,
		ChunkSize:   16,
		TotalChunks: 2,
		CreatedAt:   time.Now().UnixMilli(),
	}
	if !validUploadSession(base) {
		t.Fatal("expected valid upload session")
	}
	base.Size = 33
	if validUploadSession(base) {
		t.Fatal("expected mismatched chunk count to be rejected")
	}
	base.Size = 32
	base.CreatedAt = time.Now().Add(10 * time.Minute).UnixMilli()
	if validUploadSession(base) {
		t.Fatal("expected future session timestamp to be rejected")
	}
}

func TestCanAccessOwnedAsset(t *testing.T) {
	owner := "alice"
	if !canAccessOwnedAsset(nil, "") || !canAccessOwnedAsset(&owner, "alice") {
		t.Fatal("expected public and owner access")
	}
	if canAccessOwnedAsset(&owner, "bob") || canAccessOwnedAsset(&owner, "") {
		t.Fatal("unexpected access to another user's asset")
	}
}
