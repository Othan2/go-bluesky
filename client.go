// Copyright 2023 go-bluesky authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bluesky

import (
	"errors"
	"time"

	"github.com/bluesky-social/indigo/api/bsky"
)

var (
	// jwtAsyncRefreshThreshold is the remaining validity time of a JWT token
	// below which to trigger a session refresh on a background thread (i.e.
	// the client can still be actively used during).
	jwtAsyncRefreshThreshold = 5 * time.Minute

	// jwtSyncRefreshThreshold is the remaining validity time of a JWT token
	// below which to trigger a session refresh on a foreground thread (i.e.
	// the client blocks new API calls until the refresh finishes).
	jwtSyncRefreshThreshold = 2 * time.Minute
)

var (
	// ErrLoginUnauthorized is returned from a login attempt if the credentials
	// are rejected by the server or the local client (master credentials).
	ErrLoginUnauthorized = errors.New("unauthorized")

	// ErrMasterCredentials is returned from a login attempt if the credentials
	// are valid on the Bluesky server, but they are the user's master password.
	// Since that is a security malpractice, this library forbids it.
	ErrMasterCredentials = errors.New("Master credentials used")

	// ErrSessionExpired is returned from any API call if the underlying session
	// has expired and a new login from scratch is required.
	ErrSessionExpired = errors.New("session expired")

	// TODO: add "blusky throttled me" err
)

// Client is the interface that provides methods to interact with a Bluesky PDS instance.
// TODO: split into sub-services to better group operations and reduce clutter.
// Example grouping: profile, posts, timeline

// TODO: maybe delete? seems ok to export the concrete type.
type Client interface {
	// TODO: move this documentation to NewClient. Probably also want to move client implementation to its own file.
	// Login authenticates to the Bluesky server with the given handle and appkey.
	// Note: authenticating with a live password instead of an application key will
	// be detected and rejected. For your security, this library will refuse to use
	// your master credentials.
	// Login(ctx context.Context, handle string, appkey string) error

	// Close terminates the client, shutting down all pending tasks and background operations.
	Close() error

	// Determines whether the client is ready to start processing requests.
	Ready() bool

	// Searches bluesky for posts. https://docs.bsky.app/docs/api/app-bsky-feed-search-posts
	SearchPosts(request *SearchPostsRequest) (*bsky.FeedSearchPosts_Output, error)
}

type SearchPostsRequest struct {
	Author   string // at-identifier, format:
	Cursor   string
	Domain   string
	Lang     string
	Limit    int
	Mentions string // at-identifier, format:
	Q        string
	Since    time.Time
	Sort     string // [top, latest]
	Tag      []string
	Until    time.Time
	Url      string
}
