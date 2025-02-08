package bluesky

import (
	"context"

	"github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/indigo/xrpc"
	"github.com/rs/zerolog/log"
)

func (c *client) SearchPosts(request *SearchPostsRequest) (*bsky.FeedSearchPosts_Output, error) {
	params, err := getParamMap(request)

	if err != nil {
		return nil, err
	}

	if _, exists := params["limit"]; !exists {
		log.Warn().Msg("Did not receive a limit for SearchPosts, falling back to a default of 25.")
		params["limit"] = 25
	}

	var out bsky.FeedSearchPosts_Output
	if err := c.client.Do(context.Background(), xrpc.Query, "", "app.bsky.feed.searchPosts", params, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
