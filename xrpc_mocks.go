package bluesky

import (
	"fmt"
	"net/http"
	"sync"
)

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
