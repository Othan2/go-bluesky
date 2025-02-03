# Barebones Go client for Bluesky

[![Go Report Card](https://goreportcard.com/badge/github.com/Othan2/go-bluesky)](https://goreportcard.com/badge/github.com/Othan2/go-bluesky)

Forked from github.com/karalabe/go-bluesky. Added:

- ability to search for posts
- handling of ES256k signing algorithm for JWTs
- unit testing by injecting mock XRPC client and clocks

## Usage

```go
import "github.com/Othan2/go-bluesky"

func main() {
 client, err := bluesky.NewClient(context.Background(), "https://bsky.social", "myHandle", "myAppKey")
 
 if (err != nil) {
  panic(err)
 }

 defer client.Close()

 searchReq := SearchPostsRequest{}
 searchReq.Q = "Nathan Peterman"
 out, err := client.SearchPosts(&searchReq)
}
```

## License

3-Clause BSD
