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

package params

import "time"

// ProvisioningModel is how the provider bills an instance.
type ProvisioningModel string

const (
	// ProvisioningModelStandard is a regular, on-demand instance.
	ProvisioningModelStandard ProvisioningModel = "standard"
	// ProvisioningModelSpot is a preemptible (spot) instance.
	ProvisioningModelSpot ProvisioningModel = "spot"
)

// InstancePrice is the estimated list price of running one instance built
// from a set of bootstrap params (flavor, image and extra specs) for one hour.
// It is an estimate: discounts, taxes and network egress are not included.
type InstancePrice struct {
	// Currency is the ISO 4217 code of all prices, e.g. USD.
	Currency string `json:"currency"`
	// HourlyPrice is the sum of the hourly price of all components.
	HourlyPrice float64 `json:"hourly_price"`
	// ProvisioningModel is the provisioning model the price applies to.
	ProvisioningModel ProvisioningModel `json:"provisioning_model,omitempty"`
	// Region is the provider region the price applies to.
	Region string `json:"region,omitempty"`
	// Components is the breakdown of HourlyPrice.
	Components []InstancePriceComponent `json:"components,omitempty"`
	// Warnings lists resources that are not included in HourlyPrice
	// because the provider could not price them.
	Warnings []string `json:"warnings,omitempty"`
	// EffectiveTime is when the (newest) list prices used took effect.
	EffectiveTime time.Time `json:"effective_time,omitempty"`
}

// InstancePriceComponent is one billed resource of an instance.
type InstancePriceComponent struct {
	// Name identifies the resource, e.g. vcpu, memory, local_ssd, boot_disk.
	Name string `json:"name"`
	// Description is a human readable description, e.g. the provider SKU name.
	Description string `json:"description,omitempty"`
	// SKU is the provider's identifier of the price used.
	SKU string `json:"sku,omitempty"`
	// Quantity is the billed amount of the resource, in Unit.
	Quantity float64 `json:"quantity"`
	// Unit is the unit of Quantity, e.g. vCPU, GiB, IOPS.
	Unit string `json:"unit,omitempty"`
	// HourlyPrice is the price of Quantity for one hour.
	HourlyPrice float64 `json:"hourly_price"`
}
