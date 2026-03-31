// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License. See LICENSE in the project root for license information.

package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"golang.org/x/oauth2"
	oauth2gh "golang.org/x/oauth2/github"
)

const (
	sessionCookieName  = "waza_session"
	sessionDuration    = 24 * time.Hour
	stateCookieName    = "waza_oauth_state"
	stateCookieMaxAge  = 600 // 10 minutes
	githubUserEndpoint = "https://api.github.com/user"
)

// gitHubAPIUser is the JSON shape returned by the GitHub /user endpoint.
type gitHubAPIUser struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

// GitHubProvider implements AuthProvider using GitHub OAuth.
type GitHubProvider struct {
	oauthConfig *oauth2.Config
	jwtSecret   []byte
	store       Store

	// In-memory session revocation set (token → revoked).
	// A production deployment would use a shared store (Redis, Cosmos, etc.).
	mu       sync.RWMutex
	revoked  map[string]struct{}
}

// Store is the subset of db.Store that GitHubProvider needs for user persistence.
// This avoids an import cycle between auth and db.
type Store interface {
	CreateUser(ctx context.Context, user *User) error
	GetUser(ctx context.Context, githubID int64) (*User, error)
}

// NewGitHubProvider creates a GitHubProvider from the supplied OAuth config.
func NewGitHubProvider(cfg GitHubOAuthConfig, jwtSecret string, store Store) *GitHubProvider {
	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = []string{"read:user", "repo"}
	}
	return &GitHubProvider{
		oauthConfig: &oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.RedirectURL,
			Scopes:       scopes,
			Endpoint:     oauth2gh.Endpoint,
		},
		jwtSecret: []byte(jwtSecret),
		store:     store,
		revoked:   make(map[string]struct{}),
	}
}

// HandleLogin redirects the user to GitHub's OAuth authorize URL.
func (p *GitHubProvider) HandleLogin(w http.ResponseWriter, r *http.Request) {
	state, err := generateState()
	if err != nil {
		http.Error(w, "failed to generate state", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    state,
		MaxAge:   stateCookieMaxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})

	url := p.oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOnline)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

// HandleCallback processes the OAuth callback from GitHub, exchanges the
// authorization code for tokens, creates or updates the user record, and
// sets a session cookie.
func (p *GitHubProvider) HandleCallback(w http.ResponseWriter, r *http.Request) {
	// Validate state parameter
	stateCookie, err := r.Cookie(stateCookieName)
	if err != nil || stateCookie.Value == "" {
		http.Error(w, "missing oauth state", http.StatusBadRequest)
		return
	}
	if r.URL.Query().Get("state") != stateCookie.Value {
		http.Error(w, "invalid oauth state", http.StatusBadRequest)
		return
	}

	// Clear the state cookie
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    "",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})

	// Exchange code for token
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code parameter", http.StatusBadRequest)
		return
	}

	token, err := p.oauthConfig.Exchange(r.Context(), code)
	if err != nil {
		http.Error(w, "oauth exchange failed", http.StatusInternalServerError)
		return
	}

	// Fetch GitHub user profile
	ghUser, err := p.fetchGitHubUser(r.Context(), token)
	if err != nil {
		http.Error(w, "failed to fetch github user", http.StatusInternalServerError)
		return
	}

	// Upsert user in store
	user := &User{
		GitHubID:  ghUser.ID,
		Login:     ghUser.Login,
		Name:      ghUser.Name,
		AvatarURL: ghUser.AvatarURL,
		CreatedAt: time.Now().UTC(),
	}

	if err := p.store.CreateUser(r.Context(), user); err != nil {
		http.Error(w, "failed to save user", http.StatusInternalServerError)
		return
	}

	// Create JWT session token (includes GitHub access token for API calls)
	sessionToken, err := p.createSessionToken(user, token.AccessToken)
	if err != nil {
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionToken,
		MaxAge:   int(sessionDuration.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})

	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

// ValidateSession checks the JWT token and returns the associated user.
func (p *GitHubProvider) ValidateSession(ctx context.Context, token string) (*User, error) {
	p.mu.RLock()
	_, revoked := p.revoked[token]
	p.mu.RUnlock()
	if revoked {
		return nil, fmt.Errorf("session revoked")
	}

	claims, err := p.validateSessionToken(token)
	if err != nil {
		return nil, err
	}

	user, err := p.store.GetUser(ctx, claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("looking up user: %w", err)
	}
	if user == nil {
		return nil, fmt.Errorf("user not found")
	}

	user.GitHubToken = claims.GitHubToken
	return user, nil
}

// GetUser retrieves a user by their GitHub ID.
func (p *GitHubProvider) GetUser(ctx context.Context, githubID int64) (*User, error) {
	return p.store.GetUser(ctx, githubID)
}

// RevokeSession marks a session token as revoked.
func (p *GitHubProvider) RevokeSession(_ context.Context, token string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.revoked[token] = struct{}{}
	return nil
}

// NewAuthMiddleware returns an auth.Middleware that extracts the session token
// from the Authorization header (Bearer scheme) or the session cookie, validates
// it, and injects the authenticated User into the request context.
func NewAuthMiddleware(provider AuthProvider) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractToken(r)
			if token == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			user, err := provider.ValidateSession(r.Context(), token)
			if err != nil {
				http.Error(w, "invalid session", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), UserContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// extractToken reads the session token from the Authorization header (Bearer)
// or falls back to the session cookie.
func extractToken(r *http.Request) string {
	// Try Authorization header first
	if auth := r.Header.Get("Authorization"); len(auth) > 7 && auth[:7] == "Bearer " {
		return auth[7:]
	}
	// Fall back to cookie
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		return cookie.Value
	}
	return ""
}

// fetchGitHubUser calls the GitHub API to get the authenticated user's profile.
func (p *GitHubProvider) fetchGitHubUser(ctx context.Context, token *oauth2.Token) (*gitHubAPIUser, error) {
	client := p.oauthConfig.Client(ctx, token)

	resp, err := client.Get(githubUserEndpoint)
	if err != nil {
		return nil, fmt.Errorf("github api request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api returned %d", resp.StatusCode)
	}

	var ghUser gitHubAPIUser
	if err := json.NewDecoder(resp.Body).Decode(&ghUser); err != nil {
		return nil, fmt.Errorf("decoding github user: %w", err)
	}

	return &ghUser, nil
}

// sessionClaims is the payload embedded in the JWT session token.
type sessionClaims struct {
	UserID      int64  `json:"sub"`
	Login       string `json:"login"`
	GitHubToken string `json:"ght,omitempty"`
	IssuedAt    int64  `json:"iat"`
	ExpiresAt   int64  `json:"exp"`
}

// createSessionToken builds an HMAC-SHA256 signed JWT for the given user.
func (p *GitHubProvider) createSessionToken(user *User, githubToken string) (string, error) {
	now := time.Now().UTC()
	claims := sessionClaims{
		UserID:      user.GitHubID,
		Login:       user.Login,
		GitHubToken: githubToken,
		IssuedAt:    now.Unix(),
		ExpiresAt:   now.Add(sessionDuration).Unix(),
	}

	// Header
	header := base64Encode([]byte(`{"alg":"HS256","typ":"JWT"}`))

	// Payload
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshaling claims: %w", err)
	}
	encodedPayload := base64Encode(payload)

	// Signature
	signingInput := header + "." + encodedPayload
	sig := signHMAC(p.jwtSecret, []byte(signingInput))
	encodedSig := base64Encode(sig)

	return signingInput + "." + encodedSig, nil
}

// validateSessionToken verifies the JWT signature and expiration.
func (p *GitHubProvider) validateSessionToken(tokenStr string) (*sessionClaims, error) {
	parts := splitToken(tokenStr)
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	// Verify signature
	signingInput := parts[0] + "." + parts[1]
	expectedSig := signHMAC(p.jwtSecret, []byte(signingInput))
	actualSig, err := base64Decode(parts[2])
	if err != nil {
		return nil, fmt.Errorf("decoding signature: %w", err)
	}
	if !hmac.Equal(expectedSig, actualSig) {
		return nil, fmt.Errorf("invalid signature")
	}

	// Decode claims
	claimsJSON, err := base64Decode(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decoding claims: %w", err)
	}

	var claims sessionClaims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return nil, fmt.Errorf("parsing claims: %w", err)
	}

	if time.Now().UTC().Unix() > claims.ExpiresAt {
		return nil, fmt.Errorf("token expired")
	}

	return &claims, nil
}

func generateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func signHMAC(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func base64Encode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func base64Decode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

func splitToken(token string) []string {
	var parts []string
	start := 0
	for i := range len(token) {
		if token[i] == '.' {
			parts = append(parts, token[start:i])
			start = i + 1
		}
	}
	parts = append(parts, token[start:])
	return parts
}
