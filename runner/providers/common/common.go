// Copyright 2025 Cloudbase Solutions SRL
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
	"encoding/json"
	"fmt"
	"math"
	"strings"

	garmErrors "github.com/cloudbase/garm-provider-common/errors"
	commonParams "github.com/cloudbase/garm-provider-common/params"
	runnerCommon "github.com/cloudbase/garm/runner/common"
	"github.com/cloudbase/garm/runner/providers/util"
)

func ValidateResult(inst commonParams.ProviderInstance) error {
	if inst.ProviderID == "" {
		return garmErrors.NewProviderError("missing provider ID")
	}

	if inst.Name == "" {
		return garmErrors.NewProviderError("missing instance name")
	}

	if !util.IsValidProviderStatus(inst.Status) {
		return garmErrors.NewProviderError("invalid status returned (%s)", inst.Status)
	}

	return nil
}

// unsupportedPriceMarkers are printed by provider binaries (built with
// garm-provider-common) that do not know the GetInstancePrice command.
var unsupportedPriceMarkers = []string{
	"unknown GARM_COMMAND",
	"invalid command",
	"does not support GetInstancePrice",
}

// PriceResultFromExec decodes the output of the GetInstancePrice command.
// Providers that don't support the command yield runnerCommon.ErrPriceNotSupported.
func PriceResultFromExec(out []byte, execErr error) (commonParams.InstancePrice, error) {
	if execErr != nil {
		for _, marker := range unsupportedPriceMarkers {
			if strings.Contains(execErr.Error(), marker) {
				return commonParams.InstancePrice{}, fmt.Errorf("%w: %s", runnerCommon.ErrPriceNotSupported, execErr)
			}
		}
		return commonParams.InstancePrice{}, garmErrors.NewProviderError("provider binary returned error: %s", execErr)
	}
	var price commonParams.InstancePrice
	if err := json.Unmarshal(out, &price); err != nil {
		return commonParams.InstancePrice{}, garmErrors.NewProviderError("failed to decode response from binary: %s", err)
	}
	if price.Currency == "" {
		return commonParams.InstancePrice{}, garmErrors.NewProviderError("missing currency in price")
	}
	if price.HourlyPrice < 0 || math.IsNaN(price.HourlyPrice) || math.IsInf(price.HourlyPrice, 0) {
		return commonParams.InstancePrice{}, garmErrors.NewProviderError("invalid hourly price %v", price.HourlyPrice)
	}
	return price, nil
}
