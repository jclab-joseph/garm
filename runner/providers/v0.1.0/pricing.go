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

package v010

import (
	"context"
	"encoding/json"
	"fmt"

	commonExecution "github.com/cloudbase/garm-provider-common/execution/common"
	commonParams "github.com/cloudbase/garm-provider-common/params"
	"github.com/cloudbase/garm/runner/common"
	commonExternal "github.com/cloudbase/garm/runner/providers/common"
)

var _ common.PriceEstimator = (*external)(nil)

// GetInstancePrice runs the optional GetInstancePrice command of the
// provider binary.
func (e *external) GetInstancePrice(ctx context.Context, bootstrapParams commonParams.BootstrapInstance, base common.ProviderBaseParams) (commonParams.InstancePrice, error) {
	asEnv := []string{
		fmt.Sprintf("GARM_COMMAND=%s", commonExecution.GetInstancePriceCommand),
		fmt.Sprintf("GARM_CONTROLLER_ID=%s", e.controllerID),
		fmt.Sprintf("GARM_POOL_ID=%s", bootstrapParams.PoolID),
		fmt.Sprintf("GARM_PROVIDER_CONFIG_FILE=%s", e.cfg.External.ConfigFile),
	}
	asEnv = append(asEnv, e.environmentVariables...)

	asJs, err := json.Marshal(bootstrapParams)
	if err != nil {
		return commonParams.InstancePrice{}, fmt.Errorf("error serializing bootstrap params: %w", err)
	}

	e.recordOp("GetInstancePrice", base)
	out, execErr := e.execWithTimeout(ctx, "GetInstancePrice", base, asJs, asEnv)
	price, err := commonExternal.PriceResultFromExec(out, execErr)
	if err != nil {
		e.recordOpFailure("GetInstancePrice", base)
		return commonParams.InstancePrice{}, err
	}
	return price, nil
}
