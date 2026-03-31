// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License. See LICENSE in the project root for license information.

package db

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
	"github.com/microsoft/waza/internal/platform/auth"
)

const (
	databaseName         = "waza-platform"
	usersContainer       = "users"
	connectionsContainer = "connections"
	runRequestsContainer = "run-requests"
	resultsContainer     = "results"
	settingsContainer    = "settings"
)

// CosmosStore implements Store using Azure Cosmos DB (serverless).
// Partition key for users/connections/runs is the user's GitHub ID as a string.
// Settings use a fixed "global" partition key.
type CosmosStore struct {
	client        *azcosmos.Client
	encryptionKey []byte
	users         *azcosmos.ContainerClient
	connections   *azcosmos.ContainerClient
	runRequests   *azcosmos.ContainerClient
	results       *azcosmos.ContainerClient
	settings      *azcosmos.ContainerClient
}

// NewCosmosStore creates a CosmosStore connected to the given Cosmos DB endpoint.
// encryptionKeyB64 is a base64-encoded 32-byte AES-256 key for encrypting connection configs.
func NewCosmosStore(endpoint, accountKey, encryptionKeyB64 string) (*CosmosStore, error) {
	cred, err := azcosmos.NewKeyCredential(accountKey)
	if err != nil {
		return nil, fmt.Errorf("creating cosmos credential: %w", err)
	}

	client, err := azcosmos.NewClientWithKey(endpoint, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("creating cosmos client: %w", err)
	}

	encKey, err := base64.StdEncoding.DecodeString(encryptionKeyB64)
	if err != nil {
		return nil, fmt.Errorf("decoding encryption key: %w", err)
	}
	if len(encKey) != 32 {
		return nil, fmt.Errorf("encryption key must be 32 bytes (got %d)", len(encKey))
	}

	db, err := client.NewDatabase(databaseName)
	if err != nil {
		return nil, fmt.Errorf("getting database: %w", err)
	}

	users, err := db.NewContainer(usersContainer)
	if err != nil {
		return nil, fmt.Errorf("getting users container: %w", err)
	}

	connections, err := db.NewContainer(connectionsContainer)
	if err != nil {
		return nil, fmt.Errorf("getting connections container: %w", err)
	}

	runRequests, err := db.NewContainer(runRequestsContainer)
	if err != nil {
		return nil, fmt.Errorf("getting run-requests container: %w", err)
	}

	results, err := db.NewContainer(resultsContainer)
	if err != nil {
		return nil, fmt.Errorf("getting results container: %w", err)
	}

	settings, err := db.NewContainer(settingsContainer)
	if err != nil {
		return nil, fmt.Errorf("getting settings container: %w", err)
	}

	return &CosmosStore{
		client:        client,
		encryptionKey: encKey,
		users:         users,
		connections:   connections,
		runRequests:   runRequests,
		results:       results,
		settings:      settings,
	}, nil
}

// NewCosmosStoreWithIdentity creates a CosmosStore using DefaultAzureCredential (Entra ID / managed identity).
// Use this when Cosmos DB has disableLocalAuth: true.
func NewCosmosStoreWithIdentity(endpoint, encryptionKeyB64 string) (*CosmosStore, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("creating default azure credential: %w", err)
	}

	client, err := azcosmos.NewClient(endpoint, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("creating cosmos client with identity: %w", err)
	}

	encKey, err := base64.StdEncoding.DecodeString(encryptionKeyB64)
	if err != nil {
		return nil, fmt.Errorf("decoding encryption key: %w", err)
	}
	if len(encKey) != 32 {
		return nil, fmt.Errorf("encryption key must be 32 bytes (got %d)", len(encKey))
	}

	db, err := client.NewDatabase(databaseName)
	if err != nil {
		return nil, fmt.Errorf("getting database: %w", err)
	}

	users, err := db.NewContainer(usersContainer)
	if err != nil {
		return nil, fmt.Errorf("getting users container: %w", err)
	}

	connections, err := db.NewContainer(connectionsContainer)
	if err != nil {
		return nil, fmt.Errorf("getting connections container: %w", err)
	}

	runRequests, err := db.NewContainer(runRequestsContainer)
	if err != nil {
		return nil, fmt.Errorf("getting run-requests container: %w", err)
	}

	results, err := db.NewContainer(resultsContainer)
	if err != nil {
		return nil, fmt.Errorf("getting results container: %w", err)
	}

	settings, err := db.NewContainer(settingsContainer)
	if err != nil {
		return nil, fmt.Errorf("getting settings container: %w", err)
	}

	return &CosmosStore{
		client:        client,
		encryptionKey: encKey,
		users:         users,
		connections:   connections,
		runRequests:   runRequests,
		results:       results,
		settings:      settings,
	}, nil
}

func userPartitionKey(githubID int64) azcosmos.PartitionKey {
	return azcosmos.NewPartitionKeyString(strconv.FormatInt(githubID, 10))
}

// cosmosDoc wraps any value with the required Cosmos DB "id" field
// and a string-typed partition key field for consistent matching.
type cosmosDoc struct {
	ID          string `json:"id"`
	GitHubIDStr string `json:"github_id"` // string to match partition key path
}

// --- Users ---

func (s *CosmosStore) CreateUser(ctx context.Context, user *auth.User) error {
	// Build a document with the required "id" field and string partition key.
	doc := map[string]any{
		"id":         strconv.FormatInt(user.GitHubID, 10),
		"github_id":  strconv.FormatInt(user.GitHubID, 10),
		"github_id_n": user.GitHubID,
		"login":      user.Login,
		"name":       user.Name,
		"avatar_url": user.AvatarURL,
		"created_at": user.CreatedAt,
	}

	data, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshaling user: %w", err)
	}

	pk := userPartitionKey(user.GitHubID)
	_, err = s.users.UpsertItem(ctx, pk, data, nil)
	if err != nil {
		return fmt.Errorf("upserting user: %w", err)
	}
	return nil
}

func (s *CosmosStore) GetUser(ctx context.Context, githubID int64) (*auth.User, error) {
	pk := userPartitionKey(githubID)
	id := strconv.FormatInt(githubID, 10)

	resp, err := s.users.ReadItem(ctx, pk, id, nil)
	if err != nil {
		// Cosmos returns 404 for not-found; return nil per interface contract.
		return nil, nil //nolint:nilerr // not-found is expected
	}

	// Parse the raw document which has string github_id.
	var doc map[string]any
	if err := json.Unmarshal(resp.Value, &doc); err != nil {
		return nil, fmt.Errorf("unmarshaling user: %w", err)
	}

	user := &auth.User{
		GitHubID:  githubID,
		Login:     stringVal(doc, "login"),
		Name:      stringVal(doc, "name"),
		AvatarURL: stringVal(doc, "avatar_url"),
	}
	if ts, ok := doc["created_at"].(string); ok {
		user.CreatedAt, _ = time.Parse(time.RFC3339, ts)
	}
	return user, nil
}

func stringVal(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// --- Connections ---

func (s *CosmosStore) SaveConnection(ctx context.Context, conn *Connection) error {
	// Build document with string user_id to match partition key path.
	doc := map[string]any{
		"id":          conn.ID,
		"user_id":     strconv.FormatInt(conn.UserID, 10),
		"user_id_n":   conn.UserID,
		"type":        conn.Type,
		"config":      conn.Config,
		"verified_at": conn.VerifiedAt,
	}

	data, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshaling connection: %w", err)
	}

	pk := userPartitionKey(conn.UserID)
	_, err = s.connections.UpsertItem(ctx, pk, data, nil)
	if err != nil {
		return fmt.Errorf("upserting connection: %w", err)
	}
	return nil
}

func (s *CosmosStore) ListConnections(ctx context.Context, userID int64, connType ConnectionType) ([]*Connection, error) {
	pk := userPartitionKey(userID)
	uid := strconv.FormatInt(userID, 10)

	query := "SELECT * FROM c WHERE c.user_id = @uid"
	params := []azcosmos.QueryParameter{
		{Name: "@uid", Value: uid},
	}
	if connType != "" {
		query += " AND c.type = @type"
		params = append(params, azcosmos.QueryParameter{Name: "@type", Value: string(connType)})
	}

	slog.Info("ListConnections query", "query", query, "uid", uid, "connType", connType)

	pager := s.connections.NewQueryItemsPager(query, pk, &azcosmos.QueryOptions{
		QueryParameters: params,
	})

	var connections []*Connection
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("querying connections: %w", err)
		}
		for _, item := range page.Items {
			conn, err := parseConnection(item, userID)
			if err != nil {
				return nil, fmt.Errorf("unmarshaling connection: %w", err)
			}
			connections = append(connections, conn)
		}
	}
	return connections, nil
}

// parseConnection handles the string user_id stored in Cosmos.
func parseConnection(data []byte, fallbackUserID int64) (*Connection, error) {
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}

	conn := &Connection{
		ID:     stringVal(doc, "id"),
		UserID: fallbackUserID,
		Type:   ConnectionType(stringVal(doc, "type")),
	}

	if cfg, ok := doc["config"].(map[string]any); ok {
		conn.Config = cfg
	}

	if va, ok := doc["verified_at"].(string); ok && va != "" {
		t, _ := time.Parse(time.RFC3339, va)
		conn.VerifiedAt = &t
	}
	return conn, nil
}

func (s *CosmosStore) DeleteConnection(ctx context.Context, userID int64, connectionID string) error {
	pk := userPartitionKey(userID)
	_, err := s.connections.DeleteItem(ctx, pk, connectionID, nil)
	if err != nil {
		return fmt.Errorf("deleting connection %s: %w", connectionID, err)
	}
	return nil
}

// --- Runs ---

func (s *CosmosStore) CreateRunRequest(ctx context.Context, run *RunRequest) error {
	now := time.Now().UTC()
	run.CreatedAt = now
	run.Status = Queued

	doc := map[string]any{
		"id":              run.ID,
		"user_id":         strconv.FormatInt(run.UserID, 10),
		"user_id_n":       run.UserID,
		"repo":            run.Repo,
		"eval_spec":       run.EvalSpec,
		"model":           run.Model,
		"workers":         run.Workers,
		"status":          run.Status,
		"adc_sandbox_ids": run.ADCSandboxIDs,
		"created_at":      run.CreatedAt,
		"completed_at":    run.CompletedAt,
	}

	data, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshaling run request: %w", err)
	}

	pk := userPartitionKey(run.UserID)
	_, err = s.runRequests.CreateItem(ctx, pk, data, nil)
	if err != nil {
		return fmt.Errorf("creating run request: %w", err)
	}
	return nil
}

func (s *CosmosStore) UpdateRunRequest(ctx context.Context, run *RunRequest) error {
	doc := map[string]any{
		"id":              run.ID,
		"user_id":         strconv.FormatInt(run.UserID, 10),
		"user_id_n":       run.UserID,
		"repo":            run.Repo,
		"eval_spec":       run.EvalSpec,
		"model":           run.Model,
		"workers":         run.Workers,
		"status":          run.Status,
		"adc_sandbox_ids": run.ADCSandboxIDs,
		"created_at":      run.CreatedAt,
		"completed_at":    run.CompletedAt,
	}

	data, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshaling run request: %w", err)
	}

	pk := userPartitionKey(run.UserID)
	_, err = s.runRequests.ReplaceItem(ctx, pk, run.ID, data, nil)
	if err != nil {
		return fmt.Errorf("updating run request %s: %w", run.ID, err)
	}
	return nil
}

func (s *CosmosStore) ListRunRequests(ctx context.Context, userID int64, limit int) ([]*RunRequest, error) {
	pk := userPartitionKey(userID)
	uid := strconv.FormatInt(userID, 10)

	query := "SELECT * FROM c WHERE c.user_id = @uid ORDER BY c.created_at DESC"
	params := []azcosmos.QueryParameter{
		{Name: "@uid", Value: uid},
	}

	pager := s.runRequests.NewQueryItemsPager(query, pk, &azcosmos.QueryOptions{
		QueryParameters: params,
	})

	var requests []*RunRequest
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("querying run requests: %w", err)
		}
		for _, item := range page.Items {
			var req RunRequest
			if err := json.Unmarshal(item, &req); err != nil {
				return nil, fmt.Errorf("unmarshaling run request: %w", err)
			}
			requests = append(requests, &req)
			if limit > 0 && len(requests) >= limit {
				return requests, nil
			}
		}
	}
	return requests, nil
}

func (s *CosmosStore) GetRunRequest(ctx context.Context, userID int64, runID string) (*RunRequest, error) {
	pk := userPartitionKey(userID)
	resp, err := s.runRequests.ReadItem(ctx, pk, runID, nil)
	if err != nil {
		return nil, fmt.Errorf("reading run request %s: %w", runID, err)
	}

	var req RunRequest
	if err := json.Unmarshal(resp.Value, &req); err != nil {
		return nil, fmt.Errorf("unmarshaling run request: %w", err)
	}
	return &req, nil
}

// --- Results ---

func (s *CosmosStore) SaveResult(ctx context.Context, userID int64, runID string, result json.RawMessage) error {
	uid := strconv.FormatInt(userID, 10)

	// Extract summary fields from the result JSON for indexed querying.
	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		return fmt.Errorf("parsing result JSON: %w", err)
	}

	doc := map[string]any{
		"id":        runID,
		"user_id":   uid,
		"user_id_n": userID,
		"run_id":    runID,
		"result":    parsed,
		"spec":      stringVal(parsed, "spec"),
		"model":     stringVal(parsed, "model"),
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	// Extract pass_rate from digest if present.
	if digest, ok := parsed["digest"].(map[string]any); ok {
		if pr, ok := digest["pass_rate"].(float64); ok {
			doc["pass_rate"] = pr
		}
	}

	data, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshaling result doc: %w", err)
	}

	pk := userPartitionKey(userID)
	_, err = s.results.UpsertItem(ctx, pk, data, nil)
	if err != nil {
		return fmt.Errorf("upserting result %s: %w", runID, err)
	}
	return nil
}

func (s *CosmosStore) GetResult(ctx context.Context, userID int64, runID string) (json.RawMessage, error) {
	pk := userPartitionKey(userID)
	resp, err := s.results.ReadItem(ctx, pk, runID, nil)
	if err != nil {
		return nil, fmt.Errorf("reading result %s: %w", runID, err)
	}

	// The stored document wraps the original result under the "result" key.
	var doc map[string]any
	if err := json.Unmarshal(resp.Value, &doc); err != nil {
		return nil, fmt.Errorf("unmarshaling result doc: %w", err)
	}

	resultData, ok := doc["result"]
	if !ok {
		return nil, fmt.Errorf("result document %s missing 'result' field", runID)
	}

	raw, err := json.Marshal(resultData)
	if err != nil {
		return nil, fmt.Errorf("re-marshaling result: %w", err)
	}
	return json.RawMessage(raw), nil
}

func (s *CosmosStore) ListResults(ctx context.Context, userID int64, limit int) ([]ResultSummary, error) {
	pk := userPartitionKey(userID)
	uid := strconv.FormatInt(userID, 10)

	query := "SELECT c.id, c.user_id, c.run_id, c.spec, c.model, c.pass_rate, c.timestamp FROM c WHERE c.user_id = @uid ORDER BY c.timestamp DESC"
	params := []azcosmos.QueryParameter{
		{Name: "@uid", Value: uid},
	}

	if limit > 0 {
		query += fmt.Sprintf(" OFFSET 0 LIMIT %d", limit)
	}

	pager := s.results.NewQueryItemsPager(query, pk, &azcosmos.QueryOptions{
		QueryParameters: params,
	})

	var summaries []ResultSummary
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("querying results: %w", err)
		}
		for _, item := range page.Items {
			summary, err := parseResultSummary(item, userID)
			if err != nil {
				return nil, fmt.Errorf("unmarshaling result summary: %w", err)
			}
			summaries = append(summaries, summary)
			if limit > 0 && len(summaries) >= limit {
				return summaries, nil
			}
		}
	}
	return summaries, nil
}

// parseResultSummary extracts a ResultSummary from a Cosmos query result row.
func parseResultSummary(data []byte, fallbackUserID int64) (ResultSummary, error) {
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return ResultSummary{}, err
	}

	s := ResultSummary{
		ID:     stringVal(doc, "id"),
		UserID: fallbackUserID,
		RunID:  stringVal(doc, "run_id"),
		Spec:   stringVal(doc, "spec"),
		Model:  stringVal(doc, "model"),
	}

	if pr, ok := doc["pass_rate"].(float64); ok {
		s.PassRate = pr
	}
	if ts, ok := doc["timestamp"].(string); ok {
		s.Timestamp, _ = time.Parse(time.RFC3339, ts)
	}
	return s, nil
}

// --- Settings ---

type settingDoc struct {
	ID    string `json:"id"`
	Value string `json:"value"`
}

func (s *CosmosStore) GetSetting(ctx context.Context, key string) (string, error) {
	pk := azcosmos.NewPartitionKeyString("global")
	resp, err := s.settings.ReadItem(ctx, pk, key, nil)
	if err != nil {
		return "", nil //nolint:nilerr // not-found → empty string per contract
	}

	var doc settingDoc
	if err := json.Unmarshal(resp.Value, &doc); err != nil {
		return "", fmt.Errorf("unmarshaling setting: %w", err)
	}
	return doc.Value, nil
}

func (s *CosmosStore) SetSetting(ctx context.Context, key, value string) error {
	doc := settingDoc{ID: key, Value: value}
	data, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshaling setting: %w", err)
	}

	pk := azcosmos.NewPartitionKeyString("global")
	_, err = s.settings.UpsertItem(ctx, pk, data, nil)
	if err != nil {
		return fmt.Errorf("upserting setting %s: %w", key, err)
	}
	return nil
}

// --- Lifecycle ---

func (s *CosmosStore) Close() error {
	// azcosmos.Client doesn't have a Close method; this is a no-op.
	return nil
}

// --- Encryption helpers (AES-256-GCM) ---

// EncryptConfig encrypts plaintext config data for storage.
func (s *CosmosStore) EncryptConfig(plaintext string) (string, error) {
	block, err := aes.NewCipher(s.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("creating gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generating nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptConfig decrypts a base64-encoded AES-256-GCM ciphertext.
func (s *CosmosStore) DecryptConfig(encrypted string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return "", fmt.Errorf("decoding ciphertext: %w", err)
	}

	block, err := aes.NewCipher(s.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("creating gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypting: %w", err)
	}

	return string(plaintext), nil
}
