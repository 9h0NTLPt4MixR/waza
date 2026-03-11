package models

// EngineType identifies the execution engine to use for evaluations.
type EngineType string

const (
	// EngineTypeMock uses a lightweight mock engine for fast iteration without API calls.
	EngineTypeMock EngineType = "mock"
	// EngineTypeCopilotSDK uses the real Copilot SDK engine for model execution.
	EngineTypeCopilotSDK EngineType = "copilot-sdk"
)
