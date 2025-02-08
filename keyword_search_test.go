package bluesky

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/bluesky-social/indigo/xrpc"
	"github.com/stretchr/testify/assert"
)

func TestKeywordSearch(t *testing.T) {
	mockTransport := newDefaultMockRoundTripper()
	// TODO: cid is based on a hash of post content so, I needed to insert a real post and cid below
	// I'd like to change this to use clearly fake data.
	mockTransport.responseMap["/xrpc/app.bsky.feed.searchPosts"] = &http.Response{
		StatusCode: 200,
		Body: io.NopCloser(strings.NewReader(`
			{
				"posts": [
					{
					"uri": "at://did:plc:7exnp2zmophaapytowdi6fa2/app.bsky.feed.post/3lhopfw2xq32c",
					"cid": "bafyreic27io7r2mt3fng5nco7xrulxpfe63sto5urr7e6sfjrtwsm7tjke",
					"author": {
						"did": "did:plc:7exnp2zmophaapytowdi6fa2",
						"handle": "seinfeldism.bsky.social",
						"displayName": "Seinfeldism",
						"avatar": "https://cdn.bsky.app/img/avatar/plain/did:plc:7exnp2zmophaapytowdi6fa2/bafkreiaomzizica66acedu75f4p2qvcdwak6bkvzjfgolc2vju2rg3ju6q@jpeg",
						"viewer": {
						"muted": false,
						"blockedBy": false
						},
						"labels": [],
						"createdAt": "2023-08-28T14:23:24.771Z"
					},
					"record": {
						"$type": "app.bsky.feed.post",
						"createdAt": "2025-02-08T18:07:05Z",
						"embed": {
						"$type": "app.bsky.embed.images",
						"images": [
							{
							"alt": "",
							"image": {
								"$type": "blob",
								"ref": {
								"$link": "bafkreih4rixyzlfmlmgz3w2qvmvocdydzy5jrvzpf5toqf2uyldrtrcx7e"
								},
								"mimeType": "image/jpeg",
								"size": 184319
							}
							}
						]
						},
						"facets": [
						{
							"features": [
							{
								"$type": "app.bsky.richtext.facet#link",
								"uri": "https://seinfeldism.com/s08e01-the-foundation.php"
							}
							],
							"index": {
							"byteEnd": 83,
							"byteStart": 60
							}
						}
						],
						"text": "Mr. Peterman: Where's my pineapple? / S08E01 The Foundation https://seinfeldism...."
					},
					"embed": {
						"$type": "app.bsky.embed.images#view",
						"images": [
						{
							"thumb": "https://cdn.bsky.app/img/feed_thumbnail/plain/did:plc:7exnp2zmophaapytowdi6fa2/bafkreih4rixyzlfmlmgz3w2qvmvocdydzy5jrvzpf5toqf2uyldrtrcx7e@jpeg",
							"fullsize": "https://cdn.bsky.app/img/feed_fullsize/plain/did:plc:7exnp2zmophaapytowdi6fa2/bafkreih4rixyzlfmlmgz3w2qvmvocdydzy5jrvzpf5toqf2uyldrtrcx7e@jpeg",
							"alt": ""
						}
						]
					},
					"replyCount": 0,
					"repostCount": 1,
					"likeCount": 3,
					"quoteCount": 0,
					"indexedAt": "2025-02-08T18:07:05.920Z",
					"viewer": {
						"threadMuted": false,
						"embeddingDisabled": false
					},
					"labels": []
					}
				],
				"cursor": "1"
			}
  		`)),
	}
	c, err := NewClient(context.Background(), ServerBskySocial, "testHandle", "testAppkey", withXrpcClient(&xrpc.Client{
		Client: &http.Client{
			Transport: mockTransport,
		},
		Host: ServerBskySocial,
	}))

	if err != nil {
		t.Fatalf("Err: %v", err)
	}

	request := SearchPostsRequest{
		Q:     "peterman",
		Limit: 1,
	}

	posts, err := c.SearchPosts(&request)

	if err != nil {
		t.Fatal("Failed to search posts")
	}

	assert.Equal(t, len(posts.Posts), 1)
}
