package bluesky

import (
	"context"

	"github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/indigo/xrpc"
)

func (c *client) SearchPosts(request *SearchPostsRequest) (*SearchPostsResponse, error) {
	var out bsky.FeedSearchPosts_Output
	params := map[string]interface{}{
		"author":   request.Author,
		"cursor":   request.Cursor,
		"domain":   request.Domain,
		"lang":     request.Lang,
		"limit":    request.Limit,
		"mentions": request.Mentions,
		"q":        request.Q,
		"since":    request.Since,
		"sort":     request.Sort,
		"tag":      request.Tag,
		"until":    request.Until,
		"url":      request.Url,
	}

	if err := c.client.Do(context.Background(), xrpc.Query, "", "app.bsky.feed.searchPosts", params, nil, &out); err != nil {
		return nil, err
	}
	// sess, err := atproto.searc(context.Background(), newClient)

	response := SearchPostsResponse{}
	return &response, nil
}
