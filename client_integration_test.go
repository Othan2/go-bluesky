//go:build integration
// +build integration

package bluesky

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/rs/zerolog/log"
)

func getEnvOrSkip(t *testing.T, envVar string) string {
	val, ok := os.LookupEnv(envVar)
	if !ok {
		fmt.Printf("%v env var not found, skipping test.'\n", envVar)
		t.Skip()
	}
	return val
}

func TestBskySocialIntegration(t *testing.T) {
	// TODO: I'd like to keep test sizes as small as possible to improve readability.
	// Can I keep a static client around that I just fetch when needed?
	// That way I am unlikely to run into BSKY throttling limits as well.
	handle := getEnvOrSkip(t, "BLUESKY_TEST_HANDLE")
	appKey := getEnvOrSkip(t, "BLUESKY_TEST_APPKEY")

	realClient, err := NewClient(context.Background(), "https://bsky.social", handle, appKey)

	if err != nil {
		t.Error(err)
	}

	searchReq := SearchPostsRequest{}
	searchReq.Q = "peterman"
	searchReq.Limit = 10
	out, err := realClient.SearchPosts(&searchReq)

	if err != nil {
		log.Err(err).Msg("failed query")
		time.Sleep(100 * time.Millisecond)
		t.Fatalf("dead")
	}

	log.Info().Msgf("Num posts: %v", len(out.Posts))
}
