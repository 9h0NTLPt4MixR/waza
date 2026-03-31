// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License. See LICENSE in the project root for license information.

// Package auth defines the authentication contracts for Waza Platform.
// The primary provider is GitHub OAuth via a GitHub App, which gives us
// both user identity and repository access tokens.
package auth

import (
	"context"
	"net/http"
	"time"
)

// User represents an authenticated GitHub user.
type User struct {
	GitHubID    int64     `json:"github_id"`
	Login       string    `json:"login"`
	Name        string    `json:"name"`
	AvatarURL   string    `json:"avatar_url"`
	CreatedAt   time.Time `json:"created_at"`
	GitHubToken string    `json:"-"` // Not serialized to JSON responses; set from session
}

// Session represents an authenticated user session.
// Sessions are stored server-side; only the token is sent to the client.
type Session struct {
	UserID    int64     `json:"user_id"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Expired reports whether the session has passed its expiration time.
func (s *Session) Expired() bool {
	return time.Now().After(s.ExpiresAt)
}

// GitHubOAuthConfig holds the credentials for a GitHub OAuth App or GitHub App.
type GitHubOAuthConfig struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RedirectURL  string `json:"redirect_url"`
	// Scopes requested during OAuth. Minimum: "read:user", "repo".
	Scopes []string `json:"scopes"`
}

// AuthProvider defines the contract for authentication and session management.
// Implementations handle the full GitHub OAuth flow and session lifecycle.
type AuthProvider interface {
	// HandleLogin initiates the OAuth flow by redirecting to GitHub.
	HandleLogin(w http.ResponseWriter, r *http.Request)

	// HandleCallback processes the OAuth callback from GitHub, exchanges the
	// authorization code for tokens, creates or updates the user record, and
	// establishes a session.
	HandleCallback(w http.ResponseWriter, r *http.Request)

	// ValidateSession checks that the given token maps to a valid, non-expired
	// session and returns the associated user. Returns an error if the session
	// is invalid or expired.
	ValidateSession(ctx context.Context, token string) (*User, error)

	// GetUser retrieves a user by their GitHub ID. Returns nil and no error if
	// the user does not exist.
	GetUser(ctx context.Context, githubID int64) (*User, error)

	// RevokeSession invalidates the session associated with the given token.
	RevokeSession(ctx context.Context, token string) error
}

// Middleware returns an HTTP middleware that validates the session token from
// the Authorization header (Bearer scheme) and injects the authenticated User
// into the request context. Requests without a valid session receive 401.
type Middleware func(next http.Handler) http.Handler

// ContextKey is the type for context keys used by auth middleware.
type ContextKey string

const (
	// UserContextKey is the context key for the authenticated User.
	UserContextKey ContextKey = "waza.user"
)

// UserFromContext extracts the authenticated user from the request context.
// Returns nil if no user is present (should not happen behind auth middleware).
func UserFromContext(ctx context.Context) *User {
	u, _ := ctx.Value(UserContextKey).(*User)
	return u
}
