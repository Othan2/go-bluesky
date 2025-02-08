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
	"strings"
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

	mockTransport := newMockRoundTripper(func(req *http.Request) (*http.Response, error) {
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
	assert.True(t, c.Ready())

	assert.Equal(t, mockTransport.calledMethods["/xrpc/com.atproto.server.describeServer"], 1)
	assert.Equal(t, mockTransport.calledMethods["/xrpc/com.atproto.server.createSession"], 1)
	assert.Equal(t, mockTransport.calledMethods["/xrpc/com.atproto.server.refreshSession"], 0)

	// Advance time and wait a bit to ensure that we don't try to refresh our session as our JWT is still valid.
	clock.time = clock.time.Add(5 * time.Hour)
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, mockTransport.calledMethods["/xrpc/com.atproto.server.refreshSession"], 0)

	c.Close()

	assert.False(t, c.Ready())
}

// Tests that the JWT token can be refreshed async if the expiration time becomes
// less than the allowed window.
func TestJWTAsyncRefresh(t *testing.T) {
	var clock = &mockClock{time: time.Now()}
	now := clock.Now()

	originalAccessJWT := getAccessJwt(now, now.Add(10*time.Minute))
	originalRefreshJWT := getRefreshJwt(now, now.Add(72*time.Hour))

	postRefreshAccessJWT := getAccessJwt(now, now.Add(24*time.Hour))
	postRefreshRefreshJWT := getRefreshJwt(now, now.Add(72*time.Hour))

	mockTransport := newMockRoundTripper(func(req *http.Request) (*http.Response, error) {
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
	assert.True(t, c.Ready())

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
	assert.False(t, c.Ready())
}

// Tests that the JWT token will be refreshed synchronously if the expiration time becomes
// less than the allowed window.
func TestJWTSyncRefresh(t *testing.T) {
	var clock = &mockClock{time: time.Now()}
	now := clock.Now()

	originalAccessJWT := getAccessJwt(now, now.Add(10*time.Minute))
	originalRefreshJWT := getRefreshJwt(now, now.Add(72*time.Hour))

	postRefreshAccessJWT := getAccessJwt(now, now.Add(24*time.Hour))
	postRefreshRefreshJWT := getRefreshJwt(now, now.Add(72*time.Hour))

	mockTransport := newMockRoundTripper(func(req *http.Request) (*http.Response, error) {
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
	assert.True(t, c.Ready())

	assert.Equal(t, mockTransport.calledMethods["/xrpc/com.atproto.server.describeServer"], 1)
	assert.Equal(t, mockTransport.calledMethods["/xrpc/com.atproto.server.createSession"], 1)
	assert.Equal(t, mockTransport.calledMethods["/xrpc/com.atproto.server.refreshSession"], 0)

	// JWT should be sync refreshed given that the access token will have one minute left of its
	// its original 10 minutes.
	clock.time = clock.time.Add(9 * time.Minute)
	time.Sleep(100 * time.Millisecond)
	assert.Greater(t, mockTransport.calledMethods["/xrpc/com.atproto.server.refreshSession"], 0)

	c.Close()
	assert.False(t, c.Ready())
}

// Tests that if even the JWT refresh token got expired, the refresher errors
// out synchronously.
func TestJWTExpiredRefresh(t *testing.T) {
	var clock = &mockClock{time: time.Now()}

	now := clock.Now()
	originalAccessJWT := getAccessJwt(now, now.Add(-10*time.Minute))
	originalRefreshJWT := getRefreshJwt(now, now.Add(-2*time.Minute))

	mockTransport := newMockRoundTripper(func(req *http.Request) (*http.Response, error) {
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
				StatusCode: 403,
				Body:       io.NopCloser(strings.NewReader(`{"error": "unauthorized"}`)),
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

	assert.True(t, c.Ready())

	assert.Equal(t, mockTransport.calledMethods["/xrpc/com.atproto.server.describeServer"], 1)
	assert.Equal(t, mockTransport.calledMethods["/xrpc/com.atproto.server.createSession"], 1)
	assert.Equal(t, mockTransport.calledMethods["/xrpc/com.atproto.server.refreshSession"], 0)

	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, mockTransport.calledMethods["/xrpc/com.atproto.server.refreshSession"], 0)

	assert.False(t, c.Ready())
}
