// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License. See LICENSE in the project root for license information.

package webapi

import (
	"context"
	"encoding/json"
	"time"

	"github.com/microsoft/waza/internal/models"
	"github.com/microsoft/waza/internal/platform/db"
)

// CosmosRunStore adapts the platform db.Store to the webapi.RunStore interface.
// This is used in platform mode when no BYOS Azure Storage is configured,
// so the dashboard can display results stored in Cosmos DB.
type CosmosRunStore struct {
	store  db.Store
	userID int64
}

// NewCosmosRunStore creates a RunStore backed by Cosmos DB for a specific user.
func NewCosmosRunStore(store db.Store, userID int64) *CosmosRunStore {
	return &CosmosRunStore{store: store, userID: userID}
}

// ListRuns returns all results from Cosmos, sorted by timestamp descending.
func (c *CosmosRunStore) ListRuns(sortField, order string) ([]RunSummary, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	summaries, err := c.store.ListResults(ctx, c.userID, 0)
	if err != nil {
		return nil, err
	}

	runs := make([]RunSummary, 0, len(summaries))
	for _, s := range summaries {
		outcome := "passed"
		if s.PassRate < 100.0 {
			outcome = "failed"
		}
		runs = append(runs, RunSummary{
			ID:        s.RunID,
			Spec:      s.Spec,
			Model:     s.Model,
			Outcome:   outcome,
			Timestamp: s.Timestamp,
			Source:    "cosmos",
		})
	}

	sortRuns(runs, sortField, order)
	return runs, nil
}

// GetRun returns a full run detail by downloading the result from Cosmos.
func (c *CosmosRunStore) GetRun(id string) (*RunDetail, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	raw, err := c.store.GetResult(ctx, c.userID, id)
	if err != nil {
		return nil, ErrRunNotFound
	}

	var outcome models.EvaluationOutcome
	if err := json.Unmarshal(raw, &outcome); err != nil {
		return nil, err
	}

	return outcomeToDetail(&outcome), nil
}

// Summary returns aggregate metrics from Cosmos results.
func (c *CosmosRunStore) Summary() (*SummaryResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	summaries, err := c.store.ListResults(ctx, c.userID, 0)
	if err != nil {
		return nil, err
	}

	resp := &SummaryResponse{
		TotalRuns: len(summaries),
	}

	if len(summaries) == 0 {
		return resp, nil
	}

	totalPassRate := 0.0
	for _, s := range summaries {
		totalPassRate += s.PassRate
	}
	resp.PassRate = totalPassRate / float64(len(summaries))

	return resp, nil
}

// Ensure CosmosRunStore satisfies RunStore.
var _ RunStore = (*CosmosRunStore)(nil)
