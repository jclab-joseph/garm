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
	"fmt"
	"time"

	commonExecution "github.com/cloudbase/garm-provider-common/execution/common"
	commonParams "github.com/cloudbase/garm-provider-common/params"
	"github.com/cloudbase/garm/runner/common"
	commonExternal "github.com/cloudbase/garm/runner/providers/common"
)

var _ common.EgressEstimator = (*external)(nil)

// GetInstanceEgress runs the optional GetInstanceEgress command of the
// provider binary.
func (e *external) GetInstanceEgress(ctx context.Context, providerID string, start, end time.Time, base common.ProviderBaseParams) (commonParams.InstanceEgress, error) {
	asEnv := []string{
		fmt.Sprintf("GARM_COMMAND=%s", commonExecution.GetInstanceEgressCommand),
		fmt.Sprintf("GARM_CONTROLLER_ID=%s", e.controllerID),
		fmt.Sprintf("GARM_INSTANCE_ID=%s", providerID),
		fmt.Sprintf("GARM_POOL_ID=%s", base.PoolInfo.ID),
		fmt.Sprintf("GARM_PROVIDER_CONFIG_FILE=%s", e.cfg.External.ConfigFile),
		fmt.Sprintf("%s=%s", commonExecution.EgressStartEnv, start.UTC().Format(time.RFC3339)),
		fmt.Sprintf("%s=%s", commonExecution.EgressEndEnv, end.UTC().Format(time.RFC3339)),
	}
	asEnv = append(asEnv, e.environmentVariables...)

	e.recordOp("GetInstanceEgress", base)
	out, execErr := e.execWithTimeout(ctx, "GetInstanceEgress", base, nil, asEnv)
	egress, err := commonExternal.EgressResultFromExec(out, execErr)
	if err != nil {
		e.recordOpFailure("GetInstanceEgress", base)
		return commonParams.InstanceEgress{}, err
	}
	return egress, nil
}
