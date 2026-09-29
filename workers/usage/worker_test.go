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

package usage

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	commonParams "github.com/cloudbase/garm-provider-common/params"
	dbMocks "github.com/cloudbase/garm/database/common/mocks"
	"github.com/cloudbase/garm/params"
	"github.com/cloudbase/garm/runner/common"
	commonMocks "github.com/cloudbase/garm/runner/common/mocks"
)

// pricedProvider is a provider that implements common.PriceEstimator.
type pricedProvider struct {
	*commonMocks.Provider
	*commonMocks.PriceEstimator
}

func newPricedProvider(t *testing.T) *pricedProvider {
	return &pricedProvider{
		Provider:       commonMocks.NewProvider(t),
		PriceEstimator: commonMocks.NewPriceEstimator(t),
	}
}

func testUsage(id, flavor string) params.InstanceUsage {
	return params.InstanceUsage{
		ID:           id,
		InstanceName: "runner-" + id,
		ProviderName: "gcp",
		PoolID:       "pool-1",
		Flavor:       flavor,
		Image:        "image",
		OSType:       commonParams.Linux,
		OSArch:       commonParams.Amd64,
		ExtraSpecs:   json.RawMessage(`{"spot":true}`),
		PriceStatus:  params.UsagePricePending,
	}
}

func TestPricePending(t *testing.T) {
	store := dbMocks.NewStore(t)
	provider := newPricedProvider(t)
	w := NewWorker(context.Background(), store, map[string]common.Provider{"gcp": provider})
	now := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	w.now = func() time.Time { return now }

	price := commonParams.InstancePrice{Currency: "USD", HourlyPrice: 0.25, ProvisioningModel: commonParams.ProvisioningModelSpot}
	store.EXPECT().ListInstanceUsagePendingPrice(mock.Anything, now.Add(-retryErrorsAfter), batchSize).
		Return([]params.InstanceUsage{testUsage("1", "c4d-standard-8"), testUsage("2", "c4d-standard-8")}, nil).Once()
	// Both records have the same price request: the provider is called once.
	provider.PriceEstimator.EXPECT().GetInstancePrice(mock.Anything, mock.MatchedBy(func(b commonParams.BootstrapInstance) bool {
		return b.Flavor == "c4d-standard-8" && b.PoolID == "pool-1" && string(b.ExtraSpecs) == `{"spot":true}` && b.Name == "runner-1"
	}), mock.MatchedBy(func(base common.ProviderBaseParams) bool {
		return base.PoolInfo.ID == "pool-1"
	})).Return(price, nil).Once()
	for _, id := range []string{"1", "2"} {
		store.EXPECT().SetInstanceUsagePrice(mock.Anything, id, mock.MatchedBy(func(p params.SetInstanceUsagePriceParams) bool {
			return p.Status == params.UsagePriceOK && p.Price != nil && p.Price.HourlyPrice == 0.25
		})).Return(nil).Once()
	}
	w.PricePending()

	// The cached price expires.
	now = now.Add(priceCacheTTL + time.Second)
	store.EXPECT().ListInstanceUsagePendingPrice(mock.Anything, mock.Anything, batchSize).
		Return([]params.InstanceUsage{testUsage("3", "c4d-standard-8")}, nil).Once()
	provider.PriceEstimator.EXPECT().GetInstancePrice(mock.Anything, mock.Anything, mock.Anything).Return(price, nil).Once()
	store.EXPECT().SetInstanceUsagePrice(mock.Anything, "3", mock.Anything).Return(nil).Once()
	w.PricePending()
}

func TestPricePendingErrors(t *testing.T) {
	store := dbMocks.NewStore(t)
	priced := newPricedProvider(t)
	unpriced := commonMocks.NewProvider(t)
	w := NewWorker(context.Background(), store, map[string]common.Provider{"gcp": priced, "lxd": unpriced})

	missing := testUsage("missing", "f")
	missing.ProviderName = "gone"
	noPrices := testUsage("no-prices", "f")
	noPrices.ProviderName = "lxd"
	oldBinary := testUsage("old-binary", "old")
	failing := testUsage("failing", "failing")

	store.EXPECT().ListInstanceUsagePendingPrice(mock.Anything, mock.Anything, batchSize).
		Return([]params.InstanceUsage{missing, noPrices, oldBinary, failing}, nil).Once()
	priced.PriceEstimator.EXPECT().GetInstancePrice(mock.Anything, mock.MatchedBy(func(b commonParams.BootstrapInstance) bool {
		return b.Flavor == "old"
	}), mock.Anything).Return(commonParams.InstancePrice{}, common.ErrPriceNotSupported).Once()
	priced.PriceEstimator.EXPECT().GetInstancePrice(mock.Anything, mock.MatchedBy(func(b commonParams.BootstrapInstance) bool {
		return b.Flavor == "failing"
	}), mock.Anything).Return(commonParams.InstancePrice{}, errors.New("catalog unavailable")).Once()

	expect := map[string]params.UsagePriceStatus{
		"missing":    params.UsagePriceError,
		"no-prices":  params.UsagePriceUnsupported,
		"old-binary": params.UsagePriceUnsupported,
		"failing":    params.UsagePriceError,
	}
	for id, status := range expect {
		store.EXPECT().SetInstanceUsagePrice(mock.Anything, id, mock.MatchedBy(func(p params.SetInstanceUsagePriceParams) bool {
			return p.Status == status && p.Price == nil && p.Error != ""
		})).Return(nil).Once()
	}
	w.PricePending()
}

func TestStartClosesOrphans(t *testing.T) {
	store := dbMocks.NewStore(t)
	w := NewWorker(context.Background(), store, nil)
	store.EXPECT().CloseOrphanedInstanceUsage(mock.Anything).Return(int64(2), nil).Once()
	store.EXPECT().ListInstanceUsagePendingPrice(mock.Anything, mock.Anything, batchSize).Return(nil, nil).Maybe()
	require.NoError(t, w.Start())
	require.NoError(t, w.Stop())
	require.NoError(t, w.Stop())
}
