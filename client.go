// Copyright 2023 go-bluesky authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bluesky

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/xrpc"
	"github.com/rs/zerolog/log"
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

	// TODO: add throttling err
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
}

// TODO: I'd like to do some file level prohibition on helpers from the time package as they
// can interfere in non-obvious ways with the mocked clock.

// Exposed for mocking clock to test jwt refresh semantics.
type clockInterface interface {
	Now() time.Time
}

type realClockImpl struct{}

func (*realClockImpl) Now() time.Time {
	return time.Now()
}

// Still need to figure out how to export the real client.
// client is the concrete implementation of the Client interface.
type client struct {
	client *xrpc.Client // Underlying XRPC transport connected to the API
	clock  clockInterface

	ready            bool               // Whether the client is ready to start commiunicating with bluesky.
	refreshLock      sync.RWMutex       // Lock protecting the following JWT auth fields
	accessJwtExpire  time.Time          // Expiration time for the current access JWT token
	refreshJwtExpire time.Time          // Expiration time for the refresh JWT token
	jwtAsyncRefresh  chan struct{}      // Channel tracking if an async refresher is running
	jwtRefresherStop chan chan struct{} // Notification channel to stop the JWT refresher
}

// Claims for ATProto. github.com/golang-jwt/jwt/v5 does not support the alg ES256K yet, which is what
// is used to encrypt the JWTs we get from BSky.e
type atProtoClaims struct {
	Scope     string `json:"scope"`
	Sub       string `json:"sub"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
	Audience  string `json:"aud"`
}

// Dial connects to a remote Bluesky server and exchanges some basic information
// to ensure the connectivity works.
func Dial(ctx context.Context, server string) (Client, error) {
	return DialWithClient(ctx, server, new(http.Client))
}

// DialWithClient connects to a remote Bluesky server using a user supplied HTTP
// client and exchanges some basic information to ensure the connectivity works.
func DialWithClient(ctx context.Context, server string, httpClient *http.Client) (Client, error) {
	// Create the XRPC client from the supplied HTTP one
	local := &xrpc.Client{
		Client: httpClient,
		Host:   server,
	}
	// Do a sanity check with the server to ensure everything works. We don't
	// really care about the response as long as we get a meaningful one.
	if _, err := atproto.ServerDescribeServer(ctx, local); err != nil {
		return nil, err
	}
	c := &client{
		client: local,
	}
	return c, nil
}

type clientOption func(*clientOptionalParams)

type clientOptionalParams struct {
	clock          clockInterface
	refresherPause time.Duration
	xrpcClient     *xrpc.Client
}

func withClock(c clockInterface) clientOption {
	return func(params *clientOptionalParams) {
		params.clock = c
	}
}

func withJwtRefresherSleepFor(duration time.Duration) clientOption {
	return func(params *clientOptionalParams) {
		params.refresherPause = duration
	}
}

func withXrpcClient(c *xrpc.Client) clientOption {
	return func(params *clientOptionalParams) {
		params.xrpcClient = c
	}
}

// NewClient creates a new client authenticated to the Bluesky server with the given handle and appkey.
//
// Note, authenticating with a live password instead of an application key will
// be detected and rejected. For your security, this library will refuse to use
// your master credentials.
func NewClient(ctx context.Context, server string, handle string, appkey string, clientOptions ...clientOption) (Client, error) {
	params := &clientOptionalParams{}
	for _, opt := range clientOptions {
		opt(params)
	}

	// Create an xRPC client for our client implementation to hold on to.
	if params.xrpcClient == nil {
		params.xrpcClient = &xrpc.Client{
			Client: new(http.Client),
			Host:   server,
		}
	}

	if params.clock == nil {
		params.clock = &realClockImpl{}
	}

	// TODO: better way to check if refresherPause is unset?
	if params.refresherPause.Microseconds() == 0 {
		params.refresherPause = 5 * time.Minute
	}

	return newClientInternal(ctx, handle, appkey, params)
}

func newClientInternal(ctx context.Context, handle string, appkey string, params *clientOptionalParams) (Client, error) {
	// Do a sanity check with the server to ensure everything works. We don't
	// really care about the response as long as we get a meaningful one.
	if _, err := atproto.ServerDescribeServer(ctx, params.xrpcClient); err != nil {
		return nil, err
	}

	// Authenticate to the Bluesky server
	sess, err := atproto.ServerCreateSession(ctx, params.xrpcClient, &atproto.ServerCreateSession_Input{
		Identifier: handle,
		Password:   appkey,
	})
	if err != nil {
		// TODO: need to handle rate limiting errors correctly here. BSky rate limits to
		// creating 300 sessions a day/ 30 per 5 min: https://docs.bsky.app/docs/advanced-guides/rate-limits#hosted-account-pds-limits
		// need to switch on err to return an appropriate error for rate limiting.
		// Might just want to return err
		return nil, fmt.Errorf("%w: %v", ErrLoginUnauthorized, err)
	}
	accessJwtClaims, err := parseAccessJwtClaims(sess.AccessJwt)
	if err != nil {
		return nil, err
	}

	refreshJwtClaims, err := parseRefreshJwtClaims(sess.RefreshJwt)
	if err != nil {
		return nil, err
	}

	// Construct the authenticated client and the JWT expiration metadata
	c := &client{
		client: params.xrpcClient,
		clock:  params.clock,
		ready:  false,
	}
	params.xrpcClient.Auth = &xrpc.AuthInfo{
		AccessJwt:  sess.AccessJwt,
		RefreshJwt: sess.RefreshJwt,
		Handle:     sess.Handle,
		Did:        sess.Did,
	}
	c.accessJwtExpire = time.Unix(accessJwtClaims.ExpiresAt, 0)
	c.refreshJwtExpire = time.Unix(refreshJwtClaims.ExpiresAt, 0)

	c.jwtAsyncRefresh = make(chan struct{}, 1) // 1 async refresher allowed concurrently
	c.jwtRefresherStop = make(chan chan struct{})
	go c.refresher(params.refresherPause)

	c.ready = true
	return c, nil
}

func (c *client) Ready() bool {
	return c.ready
}

// Close terminates the client, shutting down all pending tasks and background
// operations.
func (c *client) Close() error {
	// TODO: is there anything I need to add here? Closing xRPC client, etc...
	// any potential resource leaks? obv short term because go is GCed
	log.Info().Msg("Shutting down client...")

	if !c.ready {
		log.Info().Msg("Client not ready when shutting down.")
	}

	// If the periodical JWT refresher is running, tear it down
	if c.jwtRefresherStop != nil {
		// This path is particularly brittle and prone to the refresher not stopping.
		log.Info().Msg("Found a running JWT refresher.")
		stopc := make(chan struct{})
		c.jwtRefresherStop <- stopc
		<-stopc

		c.jwtRefresherStop = nil
	}

	c.ready = false
	return nil
}

// refresher is an infinite loop that periodically checks the validity of the JWT
// tokens and runs a refresh cycle if they are getting close to expiration.
func (c *client) refresher(pause time.Duration) {
	for {
		// Attempt to refresh the JWT token
		// do we hang if mayberefreshjwt returns an error here?
		err := c.maybeRefreshJWT()

		if err == ErrSessionExpired {
			log.Err(err).Msg("Shutting down refresher. Create a new client to continue sending requests.")
			return
		}

		// Wait until some time passes or the client is shutting down
		select {
		// TODO check out bluesky's refresh session limits. Probably want to coordinate with that.
		case <-time.After(pause):
		case stopc := <-c.jwtRefresherStop:
			log.Info().Msg("Stopping refresher.")
			stopc <- struct{}{}
			log.Info().Msg("Stopped refresher.")
			return
		}
	}
}

// maybeRefreshJWT checks the remainder validity time of the JWT token and does
// a session refresh if it is necessary. Depending on the amount of time it is
// still valid it might attempt a refresh on a background thread (permitting the
// current thread to proceed) or blocking the thread and doing a sync refresh.
func (c *client) maybeRefreshJWT() error {
	log.Info().Msg("Checking JWT for refresh.")

	var (
		now               = c.clock.Now()
		invalidRefreshJwt = c.refreshJwtExpire.Before(c.clock.Now())
		needSyncRefresh   = c.accessJwtExpire.Sub(now) < jwtSyncRefreshThreshold
		needAsyncRefresh  = c.accessJwtExpire.Sub(now) < jwtAsyncRefreshThreshold
	)

	if invalidRefreshJwt {
		// we shouldn't even attempt to refresh the JWT if our refresh token is not valid
		log.Err(ErrSessionExpired).Msg("Refresh JWT expiration in the past.")
		c.ready = false
		return ErrSessionExpired
	}

	if needSyncRefresh {
		log.Info().Msg("Access JWT expires very soon, refreshing synchronously.")
		return c.refreshJWT()
	}

	// If the JWT token is still valid enough for an async refresh, do that and
	// not block the API call for it
	if needAsyncRefresh {
		log.Info().Msg("Access JWT expires soon, refreshing asynchronously.")
		select {
		case c.jwtAsyncRefresh <- struct{}{}:
			// We're the first to attempt a background refresh, do it
			go func() {
				if err := c.refreshJWT(); err != nil {
					log.Error().Err(err).Msg("Async JWT refresh failed.")
				}
				<-c.jwtAsyncRefresh
			}()
			return nil

		default:
			// Someone else is already doing a background refresh, let them
			return nil
		}
	}

	log.Info().Msgf("Current access JWT still valid until %v, current time is %v skipping refresh.", c.accessJwtExpire, c.clock.Now())

	// We've run out of the background refresh window, block the client on a
	// synchronous refresh
	return nil
}

// refreshJWT updates the JWT token and swaps out the credentials in the client.
func (c *client) refreshJWT() error {
	c.refreshLock.Lock()
	defer c.refreshLock.Unlock()

	log.Info().Msgf("Attempting to refresh JWT. Old JWT expired in %v.", c.accessJwtExpire.Sub(c.clock.Now()))

	// If the refresh token timed out too, bad luck
	if c.clock.Now().After(c.refreshJwtExpire) {
		return fmt.Errorf("%w: refresh token was valid until %v", ErrSessionExpired, c.refreshJwtExpire)
	}

	// Create a copy of the client for the refresh request
	newClient := new(xrpc.Client)
	*newClient = *c.client
	newClient.Auth = new(xrpc.AuthInfo)
	*newClient.Auth = *c.client.Auth
	newClient.Auth.AccessJwt = newClient.Auth.RefreshJwt
	sess, err := atproto.ServerRefreshSession(context.Background(), newClient)
	if err != nil {
		// err might be transient, don't close immediately.
		// TODO: Do I need to switch on error type to determine whether to close?
		return err
	}

	refreshTokenClaims, err := parseATProtoClaims(sess.RefreshJwt)
	if err != nil {
		return err
	}
	newRefreshTokenExpirationTime := time.Unix(refreshTokenClaims.ExpiresAt, 0)

	accessTokenClaims, err := parseATProtoClaims(sess.AccessJwt)
	if err != nil {
		return err
	}
	newAccessTokenExpirationTime := time.Unix(accessTokenClaims.ExpiresAt, 0)

	log.Info().Msgf("New access token expiration: %v (in %v)", accessTokenClaims.ExpiresAt, newAccessTokenExpirationTime.Sub(c.clock.Now()))

	if newRefreshTokenExpirationTime.After(c.refreshJwtExpire) {
		log.Info().Msgf("Received a new refresh token. New refresh token expiration: %v (in %v)",
			newRefreshTokenExpirationTime, newRefreshTokenExpirationTime.Sub(c.clock.Now()))
	}
	c.client.Auth = &xrpc.AuthInfo{
		AccessJwt:  sess.AccessJwt,
		RefreshJwt: sess.RefreshJwt,
		Handle:     sess.Handle,
		Did:        sess.Did,
	}
	c.accessJwtExpire = newAccessTokenExpirationTime
	c.refreshJwtExpire = newRefreshTokenExpirationTime

	return nil
}

func parseAccessJwtClaims(jwt string) (*atProtoClaims, error) {
	claims, err := parseATProtoClaims(jwt)

	if err != nil {
		return nil, err
	}

	// TODO: I think this is correct, need to verify. Getting random ErrMasterCredentials
	// Verify and reject master credentials, sorry, no bad security practices
	if claims.Scope != "com.atproto.appPass" {
		return nil, fmt.Errorf("%w: %w", ErrLoginUnauthorized, ErrMasterCredentials)
	}

	return claims, nil
}

func parseRefreshJwtClaims(jwt string) (*atProtoClaims, error) {
	return parseATProtoClaims(jwt)
}

func parseATProtoClaims(jwt string) (*atProtoClaims, error) {
	// Parse into custom struct
	var claims atProtoClaims
	// Verify and reject master credentials, sorry, no bad security practices
	parts := strings.Split(jwt, ".")
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("error decoding payload: %v", err)
	}

	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("error parsing claims: %v", err)
	}

	// Retrieve the expirations for the current and refresh JWT tokens
	if claims.ExpiresAt == 0 {
		return nil, fmt.Errorf("Received an empty expiration ts")
	}

	return &claims, nil
}
