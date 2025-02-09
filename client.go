// Copyright 2023 go-bluesky authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bluesky

import (
	"time"

	"github.com/bluesky-social/indigo/api/bsky"
)

// Client to interact with AT Protocol PDSs.
type Client interface {
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
