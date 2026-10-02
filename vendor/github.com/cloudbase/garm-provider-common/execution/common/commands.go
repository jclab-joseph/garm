// Copyright 2023 Cloudbase Solutions SRL
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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	gErrors "github.com/cloudbase/garm-provider-common/errors"
	"github.com/cloudbase/garm-provider-common/params"
	"github.com/mattn/go-isatty"
)

type ExecutionCommand string

const (
	CreateInstanceCommand     ExecutionCommand = "CreateInstance"
	DeleteInstanceCommand     ExecutionCommand = "DeleteInstance"
	GetInstanceCommand        ExecutionCommand = "GetInstance"
	ListInstancesCommand      ExecutionCommand = "ListInstances"
	StartInstanceCommand      ExecutionCommand = "StartInstance"
	StopInstanceCommand       ExecutionCommand = "StopInstance"
	RemoveAllInstancesCommand ExecutionCommand = "RemoveAllInstances"
	GetVersionCommand         ExecutionCommand = "GetVersion"
)

// V0.1.1 commands
const (
	GetSupportedInterfaceVersionsCommand ExecutionCommand = "GetSupportedInterfaceVersions"
	ValidatePoolInfoCommand              ExecutionCommand = "ValidatePoolInfo"
	GetConfigJSONSchemaCommand           ExecutionCommand = "GetConfigJSONSchema"
	GetExtraSpecsJSONSchemaCommand       ExecutionCommand = "GetExtraSpecsJSONSchema"
)

// GetInstancePriceCommand is optional for providers (see PriceEstimator). It
// reads bootstrap params from stdin, like CreateInstance, and prints an
// InstancePrice.
const GetInstancePriceCommand ExecutionCommand = "GetInstancePrice"

// GetInstanceEgressCommand is optional for providers (see EgressEstimator).
// The instance is GARM_INSTANCE_ID and the time window is given by
// GARM_EGRESS_START and GARM_EGRESS_END (RFC 3339). It prints an
// InstanceEgress.
const GetInstanceEgressCommand ExecutionCommand = "GetInstanceEgress"

const (
	EgressStartEnv = "GARM_EGRESS_START"
	EgressEndEnv   = "GARM_EGRESS_END"
)

const (
	// ExitCodeNotFound is an exit code that indicates a Not Found error
	ExitCodeNotFound int = 30
	// ExitCodeDuplicate is an exit code that indicates a duplicate error
	ExitCodeDuplicate int = 31
)

func GetBoostrapParamsFromStdin(c ExecutionCommand) (params.BootstrapInstance, error) {
	var bootstrapParams params.BootstrapInstance
	if c == CreateInstanceCommand || c == GetInstancePriceCommand {
		if isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd()) {
			return params.BootstrapInstance{}, fmt.Errorf("%s requires data passed into stdin", c)
		}

		var data bytes.Buffer
		if _, err := io.Copy(&data, os.Stdin); err != nil {
			return params.BootstrapInstance{}, fmt.Errorf("failed to copy bootstrap params")
		}

		if data.Len() == 0 {
			return params.BootstrapInstance{}, fmt.Errorf("%s requires data passed into stdin", c)
		}

		if err := json.Unmarshal(data.Bytes(), &bootstrapParams); err != nil {
			return params.BootstrapInstance{}, fmt.Errorf("failed to decode instance params: %w", err)
		}
		if bootstrapParams.ExtraSpecs == nil {
			// Initialize ExtraSpecs as an empty JSON object
			bootstrapParams.ExtraSpecs = json.RawMessage([]byte("{}"))
		}

		return bootstrapParams, nil
	}

	// If the command is not CreateInstance, we don't need to read from stdin
	return params.BootstrapInstance{}, nil
}

func ResolveErrorToExitCode(err error) int {
	if err != nil {
		if errors.Is(err, gErrors.ErrNotFound) {
			return ExitCodeNotFound
		} else if errors.Is(err, gErrors.ErrDuplicateEntity) {
			return ExitCodeDuplicate
		}
		return 1
	}
	return 0
}

// ValidatePriceParams checks the bootstrap params of a GetInstancePrice command.
func ValidatePriceParams(bootstrapParams params.BootstrapInstance) error {
	if bootstrapParams.Flavor == "" {
		return fmt.Errorf("missing flavor in bootstrap params")
	}
	return nil
}

// RunGetInstancePrice runs the GetInstancePrice command against provider,
// which must implement PriceEstimator, and returns the price as JSON.
func RunGetInstancePrice(ctx context.Context, provider any, bootstrapParams params.BootstrapInstance) (string, error) {
	estimator, ok := provider.(PriceEstimator)
	if !ok {
		return "", fmt.Errorf("provider does not support %s", GetInstancePriceCommand)
	}
	price, err := estimator.GetInstancePrice(ctx, bootstrapParams)
	if err != nil {
		return "", fmt.Errorf("failed to get instance price: %w", err)
	}
	asJs, err := json.Marshal(price)
	if err != nil {
		return "", fmt.Errorf("failed to marshal response: %w", err)
	}
	return string(asJs), nil
}

// EgressWindowFromEnv reads the time window of a GetInstanceEgress command.
func EgressWindowFromEnv() (time.Time, time.Time, error) {
	start, err := time.Parse(time.RFC3339, os.Getenv(EgressStartEnv))
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid %s: %w", EgressStartEnv, err)
	}
	end, err := time.Parse(time.RFC3339, os.Getenv(EgressEndEnv))
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid %s: %w", EgressEndEnv, err)
	}
	if !start.Before(end) {
		return time.Time{}, time.Time{}, fmt.Errorf("%s must be before %s", EgressStartEnv, EgressEndEnv)
	}
	return start, end, nil
}

// RunGetInstanceEgress runs the GetInstanceEgress command against provider,
// which must implement EgressEstimator, and returns the egress as JSON.
func RunGetInstanceEgress(ctx context.Context, provider any, instance string) (string, error) {
	estimator, ok := provider.(EgressEstimator)
	if !ok {
		return "", fmt.Errorf("provider does not support %s", GetInstanceEgressCommand)
	}
	start, end, err := EgressWindowFromEnv()
	if err != nil {
		return "", err
	}
	egress, err := estimator.GetInstanceEgress(ctx, instance, start, end)
	if err != nil {
		return "", fmt.Errorf("failed to get instance egress: %w", err)
	}
	asJs, err := json.Marshal(egress)
	if err != nil {
		return "", fmt.Errorf("failed to marshal response: %w", err)
	}
	return string(asJs), nil
}
