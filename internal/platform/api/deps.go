// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License. See LICENSE in the project root for license information.

package api

import (
	"github.com/microsoft/waza/internal/platform/auth"
	"github.com/microsoft/waza/internal/platform/db"
)

// ADCDispatcher is the subset of ADC engine functionality needed by handlers.
// This avoids importing the full adc package (which depends on the ADC SDK
// that isn't in go.mod yet).
type ADCDispatcher interface {
	// Placeholder — when the ADC SDK is wired in, this interface will expose
	// Execute and Shutdown methods.
}

// Dependencies bundles all platform services required by API handlers.
// This is the single struct passed into RegisterRoutes and shared by all
// handler closures. The ADCEngine field is optional — when nil, run
// dispatch is a no-op (useful for local dev or when ADC is not configured).
type Dependencies struct {
	Store          db.Store
	Auth           auth.AuthProvider
	AuthMiddleware auth.Middleware
	ADCEngine      ADCDispatcher
}
