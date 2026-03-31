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
	data, err := json.Marshal(conn)
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

	query := "SELECT * FROM c WHERE c.user_id = @uid"
	params := []azcosmos.QueryParameter{
		{Name: "@uid", Value: userID},
	}
	if connType != "" {
		query += " AND c.type = @type"
		params = append(params, azcosmos.QueryParameter{Name: "@type", Value: string(connType)})
	}

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
			var conn Connection
			if err := json.Unmarshal(item, &conn); err != nil {
				return nil, fmt.Errorf("unmarshaling connection: %w", err)
			}
			connections = append(connections, &conn)
		}
	}
	return connections, nil
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

	data, err := json.Marshal(run)
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
	data, err := json.Marshal(run)
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

	query := "SELECT * FROM c WHERE c.user_id = @uid ORDER BY c.created_at DESC"
	params := []azcosmos.QueryParameter{
		{Name: "@uid", Value: userID},
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
