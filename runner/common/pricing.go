// Copyright 2026 Cloudbase Solutions SRL
//
//    Licensed under the Apache License, Version 2.0 (the "License"); you may
//    not use this file except in compliance with the License. You may obtain
//    a copy of the License at
//
//         http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
//    WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
//    License for the specific language governing permissions and limitations
//    under the License.

package common

import (
	"context"
	"errors"

	commonParams "github.com/cloudbase/garm-provider-common/params"
)

// ErrPriceNotSupported is returned by PriceEstimator when the provider
// binary does not implement the GetInstancePrice command.
var ErrPriceNotSupported = errors.New("provider does not support instance prices")

// PriceEstimator is implemented by providers that can estimate the hourly
// price of the instances they create. It is optional; check for it with a
// type assertion.
type PriceEstimator interface {
	// GetInstancePrice returns the estimated hourly price of an instance
	// created with bootstrapParams.
	GetInstancePrice(ctx context.Context, bootstrapParams commonParams.BootstrapInstance, base ProviderBaseParams) (commonParams.InstancePrice, error)
}
