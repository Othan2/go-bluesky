package bluesky

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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
	// map path -> response
	responseMap map[string]*http.Response

	// tracks the number of times each RPC method had been called.
	calledMethods map[string]int

	calledMethodsMutex sync.Mutex
}

func newMockRoundTripper(responseMap map[string]*http.Response) *mockRoundTripper {
	return &mockRoundTripper{responseMap: responseMap, calledMethods: make(map[string]int)}
}

// Returns a mockRoundTripper with basic session handling logic in place.
func newDefaultMockRoundTripper() *mockRoundTripper {
	responses := make(map[string]*http.Response)

	responses["/xrpc/com.atproto.server.describeServer"] = &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(`{"availableUserDomains":["bsky.social"]}`)),
	}
	responses["/xrpc/com.atproto.server.createSession"] = &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(getDefaultCreateSessionResponse())),
	}

	return newMockRoundTripper(responses)
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	m.calledMethodsMutex.Lock()
	defer m.calledMethodsMutex.Unlock()
	fmt.Printf("called method: %v\n", req.URL.Path)
	m.calledMethods[req.URL.Path] += 1

	response, found := m.responseMap[req.URL.Path]

	if !found {
		return &http.Response{
			StatusCode: 404,
			Body:       io.NopCloser(strings.NewReader(`{"error": "not found"}`)),
		}, nil
	}

	return response, nil
}
