package storage

import (
	"os"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
	"github.com/microsoft/waza/internal/models"
)

// stubAzureBlobStore creates a stub store for testing methods that don't call Azure.
func stubAzureBlobStore() *AzureBlobStore {
	return &AzureBlobStore{client: nil, containerName: "test-container"}
}

// --- sanitizePathSegment ---

func TestSanitizePathSegment(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"simple", "simple"},
		{"has/slash", "has_slash"},
		{"back\\slash", "back_slash"},
		{"with:colon", "with_colon"},
		{"has space", "has_space"},
		{"a/b\\c:d e", "a_b_c_d_e"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := sanitizePathSegment(tt.in); got != tt.want {
			t.Errorf("sanitizePathSegment(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// --- stringPtr ---

func TestStringPtr(t *testing.T) {
	p := stringPtr("hello")
	if p == nil || *p != "hello" {
		t.Errorf("stringPtr(\"hello\") = %v, want pointer to \"hello\"", p)
	}
	p2 := stringPtr("")
	if p2 == nil || *p2 != "" {
		t.Error("stringPtr(\"\") should return pointer to empty string")
	}
}

// --- getMetadata ---

func TestGetMetadata(t *testing.T) {
	val := "myval"
	meta := map[string]*string{"key": &val}

	if got := getMetadata(meta, "key"); got != "myval" {
		t.Errorf("getMetadata present key = %q, want myval", got)
	}
	if got := getMetadata(meta, "missing"); got != "" {
		t.Errorf("getMetadata missing key = %q, want empty", got)
	}
	if got := getMetadata(nil, "key"); got != "" {
		t.Errorf("getMetadata nil map = %q, want empty", got)
	}
	meta["nilval"] = nil
	if got := getMetadata(meta, "nilval"); got != "" {
		t.Errorf("getMetadata nil value = %q, want empty", got)
	}
}

// --- isCI ---

func TestIsCI(t *testing.T) {
	// Save and clear all CI env vars.
	saved := make(map[string]string)
	for _, v := range ciEnvVars {
		saved[v] = os.Getenv(v)
		os.Unsetenv(v)
	}
	t.Cleanup(func() {
		for k, v := range saved {
			if v != "" {
				os.Setenv(k, v)
			} else {
				os.Unsetenv(k)
			}
		}
	})

	if isCI() {
		t.Error("isCI() = true when all CI vars unset")
	}

	// Setting any single var should return true.
	for _, v := range ciEnvVars {
		os.Setenv(v, "1")
		if !isCI() {
			t.Errorf("isCI() = false with %s set", v)
		}
		os.Unsetenv(v)
	}
}

// --- blobToResultSummary ---

func TestBlobToResultSummary(t *testing.T) {
	abs := stubAzureBlobStore()
	ts := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)

	blob := &container.BlobItem{
		Name: stringPtr("skill-x/run-1.json"),
		Metadata: map[string]*string{
			"runid":     stringPtr("run-1"),
			"skill":     stringPtr("skill-x"),
			"model":     stringPtr("gpt-4o"),
			"passrate":  stringPtr("80.0"),
			"timestamp": stringPtr(ts.Format(time.RFC3339)),
		},
	}

	rs, err := abs.blobToResultSummary(blob)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rs.RunID != "run-1" {
		t.Errorf("RunID = %q, want run-1", rs.RunID)
	}
	if rs.Skill != "skill-x" {
		t.Errorf("Skill = %q, want skill-x", rs.Skill)
	}
	if rs.Model != "gpt-4o" {
		t.Errorf("Model = %q, want gpt-4o", rs.Model)
	}
	if rs.PassRate != 80.0 {
		t.Errorf("PassRate = %v, want 80.0", rs.PassRate)
	}
	if rs.BlobPath != "skill-x/run-1.json" {
		t.Errorf("BlobPath = %q, want skill-x/run-1.json", rs.BlobPath)
	}
}

func TestBlobToResultSummary_NilMetadata(t *testing.T) {
	abs := stubAzureBlobStore()
	blob := &container.BlobItem{Name: stringPtr("test.json"), Metadata: nil}

	_, err := abs.blobToResultSummary(blob)
	if err == nil {
		t.Error("expected error for nil metadata")
	}
}

func TestBlobToResultSummary_MissingRequiredFields(t *testing.T) {
	abs := stubAzureBlobStore()
	// Missing timestamp
	blob := &container.BlobItem{
		Name: stringPtr("test.json"),
		Metadata: map[string]*string{
			"runid": stringPtr("r1"),
		},
	}
	_, err := abs.blobToResultSummary(blob)
	if err == nil {
		t.Error("expected error for missing timestamp")
	}
}

func TestBlobToResultSummary_BadTimestamp(t *testing.T) {
	abs := stubAzureBlobStore()
	blob := &container.BlobItem{
		Name: stringPtr("test.json"),
		Metadata: map[string]*string{
			"runid":     stringPtr("r1"),
			"timestamp": stringPtr("not-a-time"),
		},
	}
	_, err := abs.blobToResultSummary(blob)
	if err == nil {
		t.Error("expected error for bad timestamp")
	}
}

func TestBlobToResultSummary_NoPassRate(t *testing.T) {
	abs := stubAzureBlobStore()
	ts := time.Now().Format(time.RFC3339)
	blob := &container.BlobItem{
		Name: stringPtr("test.json"),
		Metadata: map[string]*string{
			"runid":     stringPtr("r1"),
			"timestamp": stringPtr(ts),
		},
	}
	rs, err := abs.blobToResultSummary(blob)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rs.PassRate != 0.0 {
		t.Errorf("PassRate = %v, want 0.0 for missing passrate", rs.PassRate)
	}
}

func TestBlobToResultSummary_NilBlobName(t *testing.T) {
	abs := stubAzureBlobStore()
	ts := time.Now().Format(time.RFC3339)
	blob := &container.BlobItem{
		Name: nil,
		Metadata: map[string]*string{
			"runid":     stringPtr("r1"),
			"timestamp": stringPtr(ts),
		},
	}
	rs, err := abs.blobToResultSummary(blob)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rs.BlobPath != "" {
		t.Errorf("BlobPath = %q, want empty for nil Name", rs.BlobPath)
	}
}

// --- AzureBlobStore.outcomeToResultSummary ---

func TestAzureBlobStore_OutcomeToResultSummary(t *testing.T) {
	abs := stubAzureBlobStore()
	o := &models.EvaluationOutcome{
		RunID:       "run-42",
		SkillTested: "my/skill",
		Timestamp:   time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		Setup:       models.OutcomeSetup{ModelID: "gpt-4o"},
		Digest:      models.OutcomeDigest{TotalTests: 10, Succeeded: 7},
	}

	rs := abs.outcomeToResultSummary(o)
	if rs.RunID != "run-42" {
		t.Errorf("RunID = %q", rs.RunID)
	}
	if rs.PassRate != 70.0 {
		t.Errorf("PassRate = %v, want 70.0", rs.PassRate)
	}
	// Skill contains a slash which gets sanitized in the blob path.
	want := "my_skill/run-42.json"
	if rs.BlobPath != want {
		t.Errorf("BlobPath = %q, want %q", rs.BlobPath, want)
	}
}

func TestAzureBlobStore_OutcomeToResultSummary_ZeroTests(t *testing.T) {
	abs := stubAzureBlobStore()
	o := &models.EvaluationOutcome{
		RunID:       "empty-run",
		SkillTested: "s",
		Digest:      models.OutcomeDigest{TotalTests: 0, Succeeded: 0},
	}
	rs := abs.outcomeToResultSummary(o)
	if rs.PassRate != 0.0 {
		t.Errorf("PassRate = %v, want 0.0 for zero tests", rs.PassRate)
	}
}

// --- NewAzureBlobStore validation ---

func TestNewAzureBlobStore_MissingAccountName(t *testing.T) {
	_, err := NewAzureBlobStore(t.Context(), "", "container")
	if err == nil {
		t.Error("expected error for empty account name")
	}
}

func TestNewAzureBlobStore_MissingContainerName(t *testing.T) {
	_, err := NewAzureBlobStore(t.Context(), "account", "")
	if err == nil {
		t.Error("expected error for empty container name")
	}
}
