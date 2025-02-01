// Copyright 2023 go-bluesky authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bluesky

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/xrpc"
	"github.com/stretchr/testify/assert"
)

// mock clock to allow us to easily advance time in tests.
// Used for testing JWT refreshes.
type mockClock struct {
	time time.Time
}

func (c *mockClock) Now() time.Time {
	return c.time
}

// MockRoundTripper implements http.RoundTripper for testing
type MockRoundTripper struct {
	// ResponseFunc allows customizing the response for each request
	ResponseFunc func(req *http.Request) (*http.Response, error)

	// tracks the number of times each RPC method had been called.
	calledMethods map[string]int

	calledMethodsMutex sync.Mutex
}

func NewMockRoundTripper(ResponseFunc func(req *http.Request) (*http.Response, error)) *MockRoundTripper {
	return &MockRoundTripper{ResponseFunc: ResponseFunc, calledMethods: make(map[string]int)}
}

func (m *MockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	m.calledMethodsMutex.Lock()
	defer m.calledMethodsMutex.Unlock()
	fmt.Printf("called method: %v\n", req.URL.Path)
	m.calledMethods[req.URL.Path] += 1
	return m.ResponseFunc(req)
}

// getenvOrSkip fetches the value of env or returns the default if the value is not able to be read.
func getenvOrDefault(envVar string, defaultVal string) string {
	val, ok := os.LookupEnv(envVar)
	if !ok {
		fmt.Printf("%v env var not found, defaulting to '%v'\n", envVar, defaultVal)
		return defaultVal
	}
	return val
}

func getAccessJwt(currentTime time.Time, expiresAt time.Time) string {
	accessClaims := atProtoClaims{
		Scope:     "com.atproto.appPass",
		Sub:       "did:plc:test",
		IssuedAt:  currentTime.Unix(),
		ExpiresAt: expiresAt.Unix(),
		Audience:  "bsky.social",
	}

	// JWT claims are encoded as base64
	accessJSON, _ := json.Marshal(accessClaims)

	return fmt.Sprint("header.", base64.RawURLEncoding.EncodeToString(accessJSON), ".signature")
}

func getRefreshJwt(currentTime time.Time, expiresAt time.Time) string {
	refreshClaims := atProtoClaims{
		Scope:     "com.atproto.refresh",
		Sub:       "did:plc:test",
		IssuedAt:  currentTime.Unix(),
		ExpiresAt: expiresAt.Unix(),
		Audience:  "bsky.social",
	}

	// JWT claims are encoded as base64
	refreshJSON, _ := json.Marshal(refreshClaims)

	return fmt.Sprint("header.", base64.RawURLEncoding.EncodeToString(refreshJSON), ".signature")
}

func getCreateSessionResponse(accessJwt string, refreshJwt string) string {
	return fmt.Sprintf(`{
		"accessJwt": "%s",
		"refreshJwt": "%s",
		"handle": "test.bsky.social",
		"did": "did:plc:test"
	}`, accessJwt, refreshJwt)
}

func getRefreshSessionResponse(accessJwt string, refreshJwt string) string {
	return fmt.Sprintf(`{
		"accessJwt": "%s",
		"refreshJwt": "%s",
		"handle": "test.bsky.social",
		"did": "did:plc:test"
	}`, accessJwt, refreshJwt)
}

// Tests that the JWT token will not get refreshed if it's still valid.
func TestJWTNoopRefresh(t *testing.T) {
	var clock = &mockClock{time: time.Now()}
	now := clock.Now()

	originalAccessJWT := getAccessJwt(now, now.Add(24*time.Hour))
	originalRefreshJWT := getRefreshJwt(now, now.Add(72*time.Hour))

	mockTransport := NewMockRoundTripper(func(req *http.Request) (*http.Response, error) {
		// Return different responses based on the request path
		switch req.URL.Path {
		case "/xrpc/com.atproto.server.describeServer":
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`{"availableUserDomains":["bsky.social"]}`)),
			}, nil
		case "/xrpc/com.atproto.server.createSession":
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(getCreateSessionResponse(originalAccessJWT, originalRefreshJWT))),
			}, nil
		default:
			return &http.Response{
				StatusCode: 404,
				Body:       io.NopCloser(strings.NewReader(`{"error": "not found"}`)),
			}, nil
		}
	})

	c, err := NewClient(context.Background(), ServerBskySocial, "testHandle", "testAppKey",
		withClock(clock),
		withXrpcClient(&xrpc.Client{
			Client: &http.Client{
				Transport: mockTransport,
			},
			Host: ServerBskySocial,
		}))
	if err != nil {
		t.Fatalf("failed to create mock client: %v", err)
	}

	assert.Equal(t, mockTransport.calledMethods["/xrpc/com.atproto.server.describeServer"], 1)
	assert.Equal(t, mockTransport.calledMethods["/xrpc/com.atproto.server.createSession"], 1)
	assert.Equal(t, mockTransport.calledMethods["/xrpc/com.atproto.server.refreshSession"], 0)

	// Advance time and wait a bit to ensure that we don't try to refresh our session as our JWT is still valid.
	clock.time = clock.time.Add(5 * time.Hour)
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, mockTransport.calledMethods["/xrpc/com.atproto.server.refreshSession"], 0)

	c.Close()
}

// Tests that the JWT token can be refreshed async if the expiration time becomes
// less than the allowed window.
func TestJWTAsyncRefresh(t *testing.T) {
	var clock = &mockClock{time: time.Now()}
	now := clock.Now()

	originalAccessJWT := getAccessJwt(now, now.Add(10*time.Minute))
	originalRefreshJWT := getRefreshJwt(now, now.Add(72*time.Hour))

	now = clock.Now()
	postRefreshAccessJWT := getAccessJwt(now, now.Add(24*time.Hour))
	postRefreshRefreshJWT := getRefreshJwt(now, now.Add(72*time.Hour))

	mockTransport := NewMockRoundTripper(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/xrpc/com.atproto.server.describeServer":
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`{"availableUserDomains":["bsky.social"]}`)),
			}, nil
		case "/xrpc/com.atproto.server.createSession":
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(getCreateSessionResponse(originalAccessJWT, originalRefreshJWT))),
			}, nil
		case "/xrpc/com.atproto.server.refreshSession":
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(getRefreshSessionResponse(postRefreshAccessJWT, postRefreshRefreshJWT))),
			}, nil
		default:
			return &http.Response{
				StatusCode: 404,
				Body:       io.NopCloser(strings.NewReader(`{"error": "not found"}`)),
			}, nil
		}
	})

	c, err := NewClient(context.Background(), ServerBskySocial, "testHandle", "testAppKey",
		withClock(clock),
		withJwtRefresherSleepFor(50*time.Millisecond),
		withXrpcClient(&xrpc.Client{
			Client: &http.Client{
				Transport: mockTransport,
			},
			Host: ServerBskySocial,
		}))

	if err != nil {
		t.Fatalf("failed to create mock client: %v", err)
	}

	assert.Equal(t, mockTransport.calledMethods["/xrpc/com.atproto.server.describeServer"], 1)
	assert.Equal(t, mockTransport.calledMethods["/xrpc/com.atproto.server.createSession"], 1)
	assert.Equal(t, mockTransport.calledMethods["/xrpc/com.atproto.server.refreshSession"], 0)

	// TODO add coverage to ensure we're exercising both the sync and async refresh paths

	// advance time just past async threshold which allows for 5 minute old JWTs.
	clock.time = clock.time.Add(6 * time.Minute)
	// JWT now has 4 minutes (10 - 6) left.
	time.Sleep(100 * time.Millisecond)
	assert.Greater(t, mockTransport.calledMethods["/xrpc/com.atproto.server.refreshSession"], 0)

	c.Close()
}

// Tests that the JWT token will be refreshed synchronously if the expiration time becomes
// less than the allowed window.
func TestJWTSyncRefresh(t *testing.T) {
	var clock = &mockClock{time: time.Now()}
	now := clock.Now()

	originalAccessJWT := getAccessJwt(now, now.Add(10*time.Minute))
	originalRefreshJWT := getRefreshJwt(now, now.Add(72*time.Hour))

	now = clock.Now()
	postRefreshAccessJWT := getAccessJwt(now, now.Add(24*time.Hour))
	postRefreshRefreshJWT := getRefreshJwt(now, now.Add(72*time.Hour))

	mockTransport := NewMockRoundTripper(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/xrpc/com.atproto.server.describeServer":
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`{"availableUserDomains":["bsky.social"]}`)),
			}, nil
		case "/xrpc/com.atproto.server.createSession":
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(getCreateSessionResponse(originalAccessJWT, originalRefreshJWT))),
			}, nil
		case "/xrpc/com.atproto.server.refreshSession":
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(getRefreshSessionResponse(postRefreshAccessJWT, postRefreshRefreshJWT))),
			}, nil
		default:
			return &http.Response{
				StatusCode: 404,
				Body:       io.NopCloser(strings.NewReader(`{"error": "not found"}`)),
			}, nil
		}
	})

	c, err := NewClient(context.Background(), ServerBskySocial, "testHandle", "testAppKey",
		withClock(clock),
		withJwtRefresherSleepFor(50*time.Millisecond),
		withXrpcClient(&xrpc.Client{
			Client: &http.Client{
				Transport: mockTransport,
			},
			Host: ServerBskySocial,
		}))

	if err != nil {
		t.Fatalf("failed to create mock client: %v", err)
	}

	assert.Equal(t, mockTransport.calledMethods["/xrpc/com.atproto.server.describeServer"], 1)
	assert.Equal(t, mockTransport.calledMethods["/xrpc/com.atproto.server.createSession"], 1)
	assert.Equal(t, mockTransport.calledMethods["/xrpc/com.atproto.server.refreshSession"], 0)

	// JWT should be sync refreshed given that the access token will have one minute left of its
	// its original 10 minutes.
	// The test gets hung trying to shut down the refresher if we add 9 minutes but works if we add 6...
	// Also doesn't call refresh if we advance by 9 minutes for whatever reason.
	clock.time = clock.time.Add(9 * time.Minute)
	time.Sleep(100 * time.Millisecond)
	assert.Greater(t, mockTransport.calledMethods["/xrpc/com.atproto.server.refreshSession"], 0)

	// time.Sleep(500 * time.Millisecond)
	c.Close()

	// TODO add coverage to ensure we're exercising both the sync and async refresh paths
}

// // Tests that if even the JWT refresh token got expired, the refresher errors
// // out synchronously.
// func TestJWTExpiredRefresh(t *testing.T) {
// 	client, err := makeTestClientWithLogin(t)
// 	client.jwtCurrentExpire = time.Time{}
// 	client.jwtRefreshExpire = time.Time{}

// 	if err := client.maybeRefreshJWT(); !errors.Is(err, ErrSessionExpired) {
// 		t.Fatalf("expired session error mismatch: have %v, want %v", err, ErrSessionExpired)
// 	}
// }

// Example of using the mock client
// func TestWithMockResponses(t *testing.T) {
// 	// Create future timestamps for JWT expiration
// 	now := time.Now()
// 	accessExpires := now.Add(6 * time.Minute) // Access token expires in 2 minutes.
// 	refreshExpires := now.Add(72 * time.Hour) // Refresh token expires in 72 hours

// 	// Create JWT claims
// 	accessClaims := atProtoClaims{
// 		Scope:     "com.atproto.appPass",
// 		Sub:       "did:plc:test",
// 		IssuedAt:  now.Unix(),
// 		ExpiresAt: accessExpires.Unix(),
// 		Audience:  "bsky.social",
// 	}
// 	refreshClaims := atProtoClaims{
// 		Scope:     "com.atproto.refresh",
// 		Sub:       "did:plc:test",
// 		IssuedAt:  now.Unix(),
// 		ExpiresAt: refreshExpires.Unix(),
// 		Audience:  "bsky.social",
// 	}

// 	// JWT claims are encoded as base64
// 	accessJSON, _ := json.Marshal(accessClaims)
// 	refreshJSON, _ := json.Marshal(refreshClaims)
// 	accessJWT := "header." + base64.RawURLEncoding.EncodeToString(accessJSON) + ".signature"
// 	refreshJWT := "header." + base64.RawURLEncoding.EncodeToString(refreshJSON) + ".signature"

// 	// Mock response for createSession
// 	createSessionResponse := fmt.Sprintf(`{
// 		"accessJwt": "%s",
// 		"refreshJwt": "%s",
// 		"handle": "test.bsky.social",
// 		"did": "did:plc:test"
// 	}`, accessJWT, refreshJWT)

// 	// TODO: might need to update this to set the refresh JWT's timestamp arbitrarily
// 	// Mock response for refreshSession.
// 	refreshSessionResponse := fmt.Sprintf(`{
// 		"accessJwt": "%s",
// 		"refreshJwt": "%s",
// 		"handle": "test.bsky.social",
// 		"did": "did:plc:test"
// 	}`, accessJWT, refreshJWT)

// 	var clock = &mockClock{time: time.Now()}

// 	mockTransport := NewMockRoundTripper(func(req *http.Request) (*http.Response, error) {
// 		// Return different responses based on the request path
// 		switch req.URL.Path {
// 		case "/xrpc/com.atproto.server.describeServer":
// 			return &http.Response{
// 				StatusCode: 200,
// 				Body:       io.NopCloser(strings.NewReader(`{"availableUserDomains":["bsky.social"]}`)),
// 			}, nil
// 		case "/xrpc/com.atproto.server.createSession":
// 			return &http.Response{
// 				StatusCode: 200,
// 				Body:       io.NopCloser(strings.NewReader(createSessionResponse)),
// 			}, nil
// 		case "/xrpc/com.atproto.server.refreshSession":
// 			return &http.Response{
// 				StatusCode: 200,
// 				Body:       io.NopCloser(strings.NewReader(refreshSessionResponse)),
// 			}, nil
// 		default:
// 			return &http.Response{
// 				StatusCode: 404,
// 				Body:       io.NopCloser(strings.NewReader(`{"error": "not found"}`)),
// 			}, nil
// 		}
// 	})

// 	_, err := NewClient(context.Background(), ServerBskySocial, "testHandle", "testAppKey",
// 		withClock(clock),
// 		withRefresherPause(10*time.Millisecond),
// 		withXrpcClient(&xrpc.Client{
// 			Client: &http.Client{
// 		t.Fatalf("failed to create mock client: %v", err)
// 	}

// 	assert.Equal(t, mockTransport.calledMethods["/xrpc/com.atproto.server.describeServer"], 1)
// 	assert.Equal(t, mockTransport.calledMethods["/xrpc/com.atproto.server.createSession"], 1)
// 	assert.Equal(t, mockTransport.calledMethods["/xrpc/com.atproto.server.refreshSession"], 0)

// 	// Advance our mocked clock and wait for a bit for the refresh hook to trigger an async refresh.
// 	clock.time.Add(10 * time.Minute)
// 	time.Sleep(100 * time.Millisecond)
// 	assert.Greater(t, mockTransport.calledMethods["/xrpc/com.atproto.server.refreshSession"].Load(), int64(0))

// 	// verify that JWT was refreshed async
// 	// how do I get the refresh JWT?
// 	// ensure that the xrpc client is called.

// 	// verify that JWT was refreshed sync?
// }
