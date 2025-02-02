package bluesky

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/rs/zerolog/log"
)

func getenvOrDefault(envVar string, defaultVal string) string {
	val, ok := os.LookupEnv(envVar)
	if !ok {
		fmt.Printf("%v env var not found, defaulting to '%v'\n", envVar, defaultVal)
		return defaultVal
	}
	return val
}

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

	out, err := realClient.Search("Peterman")

	if err != nil {
		t.Fatalf("dead")
	}

	log.Info().Msgf("Num posts: %v", len(out.Posts))
	for _, post := range out.Posts {
		log.Info().Msgf("%v", *post.LikeCount)

	}
}
