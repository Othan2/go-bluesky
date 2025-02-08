package bluesky

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

func getDefaultCreateSessionResponse() string {
	accessClaims := atProtoClaims{
		Scope:     "com.atproto.appPass",
		Sub:       "did:plc:test",
		IssuedAt:  time.Now().Unix(),
		ExpiresAt: time.Now().Add(10 * time.Hour).Unix(),
		Audience:  "bsky.social",
	}

	// JWT claims are encoded as base64
	accessJSON, _ := json.Marshal(accessClaims)

	// reuse the same JWT for both access/refresh. It's a mock!
	jwt := fmt.Sprint("header.", base64.RawURLEncoding.EncodeToString(accessJSON), ".signature")

	return fmt.Sprintf(`{
		"accessJwt": "%v",
		"refreshJwt": "%v",
		"handle": "test.bsky.social",
		"did": "did:plc:test"
	}`, jwt, jwt)
}

// mockRoundTripper implements http.RoundTripper for testing
type mockRoundTripper struct {
	// responseFunc allows customizing the response for each request
	responseFunc func(req *http.Request) (*http.Response, error)

	// tracks the number of times each RPC method had been called.
	calledMethods map[string]int

	calledMethodsMutex sync.Mutex
}

func newMockRoundTripper(ResponseFunc func(req *http.Request) (*http.Response, error)) *mockRoundTripper {
	return &mockRoundTripper{responseFunc: ResponseFunc, calledMethods: make(map[string]int)}
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	m.calledMethodsMutex.Lock()
	defer m.calledMethodsMutex.Unlock()
	fmt.Printf("called method: %v\n", req.URL.Path)
	m.calledMethods[req.URL.Path] += 1
	return m.responseFunc(req)
}
