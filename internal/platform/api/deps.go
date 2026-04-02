// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License. See LICENSE in the project root for license information.

package api

import (
	"github.com/microsoft/waza/internal/platform/adc"
	"github.com/microsoft/waza/internal/platform/auth"
	"github.com/microsoft/waza/internal/platform/db"
)

// Dependencies bundles all platform services required by API handlers.
// This is the single struct passed into RegisterRoutes and shared by all
// handler closures. The ADCConfig field is optional — when nil, run
// dispatch uses the local subprocess engine.
type Dependencies struct {
	Store          db.Store
	Auth           auth.AuthProvider
	AuthMiddleware auth.Middleware
	ADCConfig      *adc.ADCConfig // nil = ADC not configured (local subprocess mode)
}
