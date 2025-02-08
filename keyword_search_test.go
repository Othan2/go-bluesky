package bluesky

import (
	"testing"
)

func TestKeywordSearch(t *testing.T) {
	// mockTransport := newMockRoundTripper(func(req *http.Request) (*http.Response, error) {
	// 	// Return different responses based on the request path
	// 	switch req.URL.Path {
	// 	case "/xrpc/com.atproto.server.describeServer":
	// 		return &http.Response{
	// 			StatusCode: 200,
	// 			Body:       io.NopCloser(strings.NewReader(`{"availableUserDomains":["bsky.social"]}`)),
	// 		}, nil
	// 	case "/xrpc/com.atproto.server.createSession":
	// 		return &http.Response{
	// 			StatusCode: 200,
	// 			Body:       io.NopCloser(strings.NewReader(getDefaultCreateSessionResponse())),
	// 		}, nil
	// 	default:
	// 		return &http.Response{
	// 			StatusCode: 404,
	// 			Body:       io.NopCloser(strings.NewReader(`{"error": "not found"}`)),
	// 		}, nil
	// 	}
	// })
}
