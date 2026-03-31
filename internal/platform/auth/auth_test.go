package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Mock auth store – satisfies auth.Store for GitHubProvider
// ---------------------------------------------------------------------------

type mockAuthStore struct {
	users map[int64]*User
}

func newMockAuthStore() *mockAuthStore {
	return &mockAuthStore{users: make(map[int64]*User)}
}

func (m *mockAuthStore) CreateUser(_ context.Context, u *User) error {
	m.users[u.GitHubID] = u
	return nil
}

func (m *mockAuthStore) GetUser(_ context.Context, githubID int64) (*User, error) {
	u, ok := m.users[githubID]
	if !ok {
		return nil, nil
	}
	return u, nil
}

var _ Store = (*mockAuthStore)(nil)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const testJWTSecret = "super-secret-test-key-32-bytes!!"

func testOAuthConfig() GitHubOAuthConfig {
	return GitHubOAuthConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURL:  "http://localhost:8080/auth/callback",
		Scopes:       []string{"read:user", "repo"},
	}
}

func testProvider() (*GitHubProvider, *mockAuthStore) {
	store := newMockAuthStore()
	p := NewGitHubProvider(testOAuthConfig(), testJWTSecret, store)
	return p, store
}

func seedUser(t *testing.T, p *GitHubProvider, store *mockAuthStore) (*User, string) {
	t.Helper()
	user := &User{
		GitHubID:  12345,
		Login:     "testuser",
		Name:      "Test User",
		AvatarURL: "https://avatars.githubusercontent.com/u/12345",
		CreatedAt: time.Now().UTC(),
	}
	store.users[user.GitHubID] = user
	token, err := p.createSessionToken(user, "test-gh-token")
	require.NoError(t, err)
	return user, token
}

// buildTestToken creates a JWT using the same HMAC-SHA256 format as GitHubProvider.
func buildTestToken(t *testing.T, secret []byte, claims sessionClaims) string {
	t.Helper()
	header := base64Encode([]byte(`{"alg":"HS256","typ":"JWT"}`))

	payload, err := json.Marshal(claims)
	require.NoError(t, err)
	encodedPayload := base64Encode(payload)

	signingInput := header + "." + encodedPayload
	sig := signHMAC(secret, []byte(signingInput))
	encodedSig := base64Encode(sig)

	return signingInput + "." + encodedSig
}

// ---------------------------------------------------------------------------
// Tests – GitHub OAuth login redirect
// ---------------------------------------------------------------------------

func TestHandleLogin_RedirectsToGitHub(t *testing.T) {
	provider, _ := testProvider()

	req := httptest.NewRequest(http.MethodGet, "/api/auth/github", nil)
	rec := httptest.NewRecorder()

	provider.HandleLogin(rec, req)

	assert.Equal(t, http.StatusTemporaryRedirect, rec.Code, "should redirect")

	loc, err := url.Parse(rec.Header().Get("Location"))
	require.NoError(t, err, "Location header must be a valid URL")

	assert.Equal(t, "github.com", loc.Host)
	assert.Contains(t, loc.Path, "/login/oauth/authorize")

	q := loc.Query()
	cfg := testOAuthConfig()
	assert.Equal(t, cfg.ClientID, q.Get("client_id"), "client_id must match config")
	assert.Equal(t, cfg.RedirectURL, q.Get("redirect_uri"), "redirect_uri must match config")
	assert.NotEmpty(t, q.Get("state"), "state param must be present for CSRF protection")

	// Verify state cookie is set
	cookies := rec.Result().Cookies()
	var stateCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == stateCookieName {
			stateCookie = c
		}
	}
	require.NotNil(t, stateCookie, "state cookie must be set")
	assert.True(t, stateCookie.HttpOnly, "state cookie must be HttpOnly")
	assert.True(t, stateCookie.Secure, "state cookie must be Secure")
	assert.Equal(t, q.Get("state"), stateCookie.Value, "state cookie must match URL state param")
}

// ---------------------------------------------------------------------------
// Tests – OAuth callback
// ---------------------------------------------------------------------------

func TestHandleCallback_ExchangesCodeForToken(t *testing.T) {
	t.Skip("requires mock GitHub OAuth token exchange server — integration test")
}

func TestHandleCallback_InvalidCode(t *testing.T) {
	t.Skip("requires mock GitHub OAuth server — integration test")
}

func TestHandleCallback_MissingState(t *testing.T) {
	provider, _ := testProvider()

	req := httptest.NewRequest(http.MethodGet, "/api/auth/callback?code=valid_code&state=some_state", nil)
	rec := httptest.NewRecorder()

	provider.HandleCallback(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code, "should reject request missing state cookie")
	assert.Contains(t, rec.Body.String(), "state", "error body should mention state")
}

func TestHandleCallback_MismatchedState(t *testing.T) {
	provider, _ := testProvider()

	req := httptest.NewRequest(http.MethodGet, "/api/auth/callback?code=valid_code&state=wrong_state", nil)
	req.AddCookie(&http.Cookie{Name: stateCookieName, Value: "correct_state"})
	rec := httptest.NewRecorder()

	provider.HandleCallback(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code, "should reject mismatched state")
	assert.Contains(t, rec.Body.String(), "state", "error body should mention state")
}

func TestHandleCallback_MissingCode(t *testing.T) {
	provider, _ := testProvider()

	req := httptest.NewRequest(http.MethodGet, "/api/auth/callback?state=valid_state", nil)
	req.AddCookie(&http.Cookie{Name: stateCookieName, Value: "valid_state"})
	rec := httptest.NewRecorder()

	provider.HandleCallback(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code, "should reject request missing code param")
}

// ---------------------------------------------------------------------------
// Tests – JWT session validation
// ---------------------------------------------------------------------------

func TestValidateSession_ValidJWT(t *testing.T) {
	provider, store := testProvider()
	user, token := seedUser(t, provider, store)

	got, err := provider.ValidateSession(context.Background(), token)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, user.Login, got.Login)
	assert.Equal(t, user.GitHubID, got.GitHubID)
}

func TestValidateSession_ExpiredJWT(t *testing.T) {
	provider, store := testProvider()
	user := &User{
		GitHubID:  99999,
		Login:     "expired-user",
		CreatedAt: time.Now().UTC(),
	}
	store.users[user.GitHubID] = user

	expiredClaims := sessionClaims{
		UserID:    user.GitHubID,
		Login:     user.Login,
		IssuedAt:  time.Now().Add(-2 * time.Hour).Unix(),
		ExpiresAt: time.Now().Add(-1 * time.Hour).Unix(),
	}
	expiredToken := buildTestToken(t, provider.jwtSecret, expiredClaims)

	got, err := provider.ValidateSession(context.Background(), expiredToken)
	assert.Error(t, err, "expired JWT must return error")
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "expired", "error should mention expiration")
}

func TestValidateSession_MissingCookie(t *testing.T) {
	provider, _ := testProvider()

	got, err := provider.ValidateSession(context.Background(), "")
	assert.Error(t, err)
	assert.Nil(t, got)
}

func TestValidateSession_InvalidSignature(t *testing.T) {
	provider, store := testProvider()
	user := &User{
		GitHubID:  12345,
		Login:     "testuser",
		CreatedAt: time.Now().UTC(),
	}
	store.users[user.GitHubID] = user

	wrongSecret := []byte("wrong-secret-key-32-bytes-long!!")
	claims := sessionClaims{
		UserID:    user.GitHubID,
		Login:     user.Login,
		IssuedAt:  time.Now().Unix(),
		ExpiresAt: time.Now().Add(24 * time.Hour).Unix(),
	}
	tamperedToken := buildTestToken(t, wrongSecret, claims)

	got, err := provider.ValidateSession(context.Background(), tamperedToken)
	assert.Error(t, err, "tampered JWT must return error")
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "signature", "error should mention signature")
}

func TestValidateSession_RevokedToken(t *testing.T) {
	provider, store := testProvider()
	_, token := seedUser(t, provider, store)

	err := provider.RevokeSession(context.Background(), token)
	require.NoError(t, err)

	got, err := provider.ValidateSession(context.Background(), token)
	assert.Error(t, err, "revoked token must return error")
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "revoked")
}

// ---------------------------------------------------------------------------
// Tests – Auth middleware (HTTP layer)
// ---------------------------------------------------------------------------

func TestAuthMiddleware_ValidToken(t *testing.T) {
	provider, store := testProvider()
	user, token := seedUser(t, provider, store)
	middleware := NewAuthMiddleware(provider)

	var capturedUser *User
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		capturedUser = UserFromContext(r.Context())
	})

	req := httptest.NewRequest(http.MethodGet, "/api/connections", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rec := httptest.NewRecorder()

	middleware(inner).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code, "valid token should pass middleware")
	require.NotNil(t, capturedUser, "user must be injected into context")
	assert.Equal(t, user.Login, capturedUser.Login)
}

func TestAuthMiddleware_MissingCookie(t *testing.T) {
	provider, _ := testProvider()
	middleware := NewAuthMiddleware(provider)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/connections", nil)
	rec := httptest.NewRecorder()

	middleware(inner).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code, "missing cookie must return 401")
}

func TestAuthMiddleware_BearerToken(t *testing.T) {
	provider, store := testProvider()
	_, token := seedUser(t, provider, store)
	middleware := NewAuthMiddleware(provider)

	var capturedUser *User
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		capturedUser = UserFromContext(r.Context())
	})

	req := httptest.NewRequest(http.MethodGet, "/api/connections", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	middleware(inner).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code, "Bearer token should work")
	require.NotNil(t, capturedUser)
}

// ---------------------------------------------------------------------------
// Tests – user context helpers
// ---------------------------------------------------------------------------

func TestGetUser_FromContext(t *testing.T) {
	u := &User{
		GitHubID:  12345,
		Login:     "testuser",
		Name:      "Test User",
		AvatarURL: "https://avatars.githubusercontent.com/u/12345",
	}

	ctx := context.WithValue(context.Background(), UserContextKey, u)

	got := UserFromContext(ctx)
	require.NotNil(t, got)
	assert.Equal(t, u.Login, got.Login)
	assert.Equal(t, u.GitHubID, got.GitHubID)
	assert.Equal(t, u.AvatarURL, got.AvatarURL)
}

func TestGetUser_FromContext_Missing(t *testing.T) {
	got := UserFromContext(context.Background())
	assert.Nil(t, got, "should return nil when no user in context")
}

func TestGetUser_FromContext_WrongType(t *testing.T) {
	ctx := context.WithValue(context.Background(), UserContextKey, "not-a-user")
	got := UserFromContext(ctx)
	assert.Nil(t, got, "should return nil when context value is wrong type")
}

// ---------------------------------------------------------------------------
// Tests – OAuth state generation
// ---------------------------------------------------------------------------

func TestOAuthState_Uniqueness(t *testing.T) {
	states := make(map[string]bool)
	for range 100 {
		s, err := generateState()
		require.NoError(t, err)
		assert.Len(t, s, 32, "state should be 16 bytes hex-encoded = 32 chars")
		assert.False(t, states[s], "state tokens must be unique")
		states[s] = true
	}
}

// ---------------------------------------------------------------------------
// Tests – Session model
// ---------------------------------------------------------------------------

func TestSession_Expired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{"future", time.Now().Add(1 * time.Hour), false},
		{"past", time.Now().Add(-1 * time.Hour), true},
		{"just_now", time.Now().Add(-1 * time.Millisecond), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Session{ExpiresAt: tt.expiresAt}
			assert.Equal(t, tt.want, s.Expired())
		})
	}
}
