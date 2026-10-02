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

// InstanceEgress is the network traffic an instance sent in a time window,
// and the estimated cost of the part that is billed.
type InstanceEgress struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
	// Currency is the ISO 4217 code of all costs, e.g. USD.
	Currency string `json:"currency,omitempty"`
	// Flows breaks the sent bytes down by destination.
	Flows []EgressFlow `json:"flows,omitempty"`
	// SentBytes is the total the instance sent, as measured by the
	// provider independently of Flows, if available (e.g. including
	// packet headers). 0 if unknown.
	SentBytes int64 `json:"sent_bytes,omitempty"`
	// Charges are the billed parts of Flows.
	Charges []EgressCharge `json:"charges,omitempty"`
	// Cost is the sum of the cost of Charges.
	Cost float64 `json:"cost"`
	// Warnings lists traffic that could not be priced.
	Warnings []string `json:"warnings,omitempty"`
}

// EgressFlow is the traffic sent to one kind of destination.
type EgressFlow struct {
	// LocationType is the provider's kind of destination, e.g. EXTERNAL
	// (internet) or CLOUD (inside the provider network).
	LocationType string `json:"location_type"`
	// Continent and Country locate external destinations, when known.
	Continent string `json:"continent,omitempty"`
	Country   string `json:"country,omitempty"`
	Bytes     int64  `json:"bytes"`
}

// EgressCharge is the estimated cost of the traffic sent to one billed
// destination.
type EgressCharge struct {
	// Destination is the billed destination, e.g. Americas.
	Destination string `json:"destination"`
	// Description is the provider's description of the price, e.g. a SKU name.
	Description string `json:"description,omitempty"`
	SKU         string `json:"sku,omitempty"`
	Bytes       int64  `json:"bytes"`
	// UnitPrice is the price of one GiB.
	UnitPrice float64 `json:"unit_price"`
	Cost      float64 `json:"cost"`
}
