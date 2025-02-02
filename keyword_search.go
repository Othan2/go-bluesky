package bluesky

import (
	"context"

	"github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/indigo/xrpc"
)

func (c *client) Search(phrase string) (*bsky.FeedSearchPosts_Output, error) {
	var out bsky.FeedSearchPosts_Output
	params := map[string]interface{}{
		// "author":   author,
		// "cursor":   cursor,
		// "domain":   domain,
		// "lang":     lang,
		// "limit":    limit,
		// "mentions": mentions,
		"q": phrase,
		// "since":    since,
		// "sort":     sort,
		// "tag":      tag,
		// "until":    until,
		// "url":      url,
	}

	if err := c.client.Do(context.Background(), xrpc.Query, "", "app.bsky.feed.searchPosts", params, nil, &out); err != nil {
		return nil, err
	}
	// sess, err := atproto.searc(context.Background(), newClient)
	return &out, nil
}
