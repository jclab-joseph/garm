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

// Package usage prices instance usage records. The records themselves are
// written by the database layer, alongside the instances; this worker looks
// up the hourly price of each new record through its provider.
package usage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	commonParams "github.com/cloudbase/garm-provider-common/params"
	dbCommon "github.com/cloudbase/garm/database/common"
	"github.com/cloudbase/garm/params"
	"github.com/cloudbase/garm/runner/common"
	garmUtil "github.com/cloudbase/garm/util"
)

const (
	// pollInterval is how often the worker looks for unpriced records.
	pollInterval = 15 * time.Second
	// retryErrorsAfter is how long a failed price lookup waits for a retry.
	retryErrorsAfter = 10 * time.Minute
	// priceCacheTTL bounds how long a looked up price is reused for other
	// instances of the same pool. List prices change rarely; Spot prices at
	// most monthly.
	priceCacheTTL = time.Hour
	// lookupTimeout bounds one provider call. The first call of a provider
	// may download a large price catalog.
	lookupTimeout = 3 * time.Minute
	batchSize     = 50
)

type cachedPrice struct {
	price     commonParams.InstancePrice
	fetchedAt time.Time
}

type Worker struct {
	ctx       context.Context
	store     dbCommon.Store
	providers map[string]common.Provider

	// cache maps a price request (see cacheKey) to its price.
	cache map[string]cachedPrice
	now   func() time.Time

	mux     sync.Mutex
	running bool
	quit    chan struct{}
	done    chan struct{}
}

func NewWorker(ctx context.Context, store dbCommon.Store, providers map[string]common.Provider) *Worker {
	ctx = garmUtil.WithSlogContext(ctx, slog.Any("worker", "usage-worker"))
	return &Worker{
		ctx:       ctx,
		store:     store,
		providers: providers,
		cache:     map[string]cachedPrice{},
		now:       time.Now,
	}
}

func (w *Worker) Start() error {
	w.mux.Lock()
	defer w.mux.Unlock()
	if w.running {
		return nil
	}

	closed, err := w.store.CloseOrphanedInstanceUsage(w.ctx)
	if err != nil {
		return fmt.Errorf("closing orphaned instance usage: %w", err)
	}
	if closed > 0 {
		slog.WarnContext(w.ctx, "closed usage records of instances that no longer exist", "count", closed)
	}

	w.running = true
	w.quit = make(chan struct{})
	w.done = make(chan struct{})
	go w.loop()
	return nil
}

func (w *Worker) Stop() error {
	w.mux.Lock()
	defer w.mux.Unlock()
	if !w.running {
		return nil
	}
	w.running = false
	close(w.quit)
	<-w.done
	return nil
}

func (w *Worker) loop() {
	defer close(w.done)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		w.PricePending()
		select {
		case <-ticker.C:
		case <-w.quit:
			return
		case <-w.ctx.Done():
			return
		}
	}
}

// PricePending looks up the price of the records that need one.
func (w *Worker) PricePending() {
	pending, err := w.store.ListInstanceUsagePendingPrice(w.ctx, w.now().Add(-retryErrorsAfter).UTC(), batchSize)
	if err != nil {
		slog.With(slog.Any("error", err)).ErrorContext(w.ctx, "failed to list unpriced instance usage")
		return
	}
	for _, usage := range pending {
		select {
		case <-w.quit:
			return
		case <-w.ctx.Done():
			return
		default:
		}
		result := w.lookup(usage)
		if err := w.store.SetInstanceUsagePrice(w.ctx, usage.ID, result); err != nil {
			slog.With(slog.Any("error", err)).ErrorContext(
				w.ctx, "failed to save instance price", "instance", usage.InstanceName)
		}
	}
}

func (w *Worker) lookup(usage params.InstanceUsage) params.SetInstanceUsagePriceParams {
	provider, ok := w.providers[usage.ProviderName]
	if !ok {
		return params.SetInstanceUsagePriceParams{
			Status: params.UsagePriceError,
			Error:  fmt.Sprintf("provider %q is not configured", usage.ProviderName),
		}
	}
	estimator, ok := provider.(common.PriceEstimator)
	if !ok {
		return params.SetInstanceUsagePriceParams{
			Status: params.UsagePriceUnsupported,
			Error:  fmt.Sprintf("provider %q can't price instances", usage.ProviderName),
		}
	}

	bootstrap := commonParams.BootstrapInstance{
		Name:       usage.InstanceName,
		PoolID:     usage.PoolID,
		Flavor:     usage.Flavor,
		Image:      usage.Image,
		OSType:     usage.OSType,
		OSArch:     usage.OSArch,
		ExtraSpecs: usage.ExtraSpecs,
	}
	key := cacheKey(usage)
	if cached, ok := w.cache[key]; ok && w.now().Sub(cached.fetchedAt) < priceCacheTTL {
		price := cached.price
		return params.SetInstanceUsagePriceParams{Status: params.UsagePriceOK, Price: &price}
	}

	base := common.ProviderBaseParams{
		PoolInfo:   params.Pool{ID: usage.PoolID},
		ScaleSetID: usage.ScaleSetID,
		EntityType: usage.EntityType,
		EntityID:   usage.EntityID,
	}
	ctx, cancel := context.WithTimeout(w.ctx, lookupTimeout)
	defer cancel()
	price, err := estimator.GetInstancePrice(ctx, bootstrap, base)
	if err != nil {
		if errors.Is(err, common.ErrPriceNotSupported) {
			return params.SetInstanceUsagePriceParams{Status: params.UsagePriceUnsupported, Error: err.Error()}
		}
		slog.With(slog.Any("error", err)).WarnContext(
			w.ctx, "failed to look up instance price", "instance", usage.InstanceName, "provider", usage.ProviderName)
		return params.SetInstanceUsagePriceParams{Status: params.UsagePriceError, Error: err.Error()}
	}
	for _, warning := range price.Warnings {
		slog.WarnContext(w.ctx, "instance price is incomplete", "instance", usage.InstanceName, "warning", warning)
	}
	w.cache[key] = cachedPrice{price: price, fetchedAt: w.now()}
	return params.SetInstanceUsagePriceParams{Status: params.UsagePriceOK, Price: &price}
}

// cacheKey identifies the price request of a record: records with the same
// provider, flavor, image and extra specs have the same price.
func cacheKey(usage params.InstanceUsage) string {
	h := sha256.New()
	for _, part := range []string{
		usage.ProviderName, usage.Flavor, usage.Image,
		string(usage.OSType), string(usage.OSArch), string(usage.ExtraSpecs),
	} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
