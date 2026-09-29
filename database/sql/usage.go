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

package sql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	runnerErrors "github.com/cloudbase/garm-provider-common/errors"
	commonParams "github.com/cloudbase/garm-provider-common/params"
	"github.com/cloudbase/garm/params"
)

// InstanceUsage records one run of an instance with its estimated price. It
// has no foreign keys: it must outlive the instance, pool and entity.
//
// The record is written in the same transactions that create, update and
// delete the instance, so it can't miss a lifecycle change.
type InstanceUsage struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key;"`
	CreatedAt time.Time
	UpdatedAt time.Time

	InstanceID   uuid.UUID `gorm:"type:uuid;index"`
	InstanceName string    `gorm:"index"`
	ProviderID   string
	ProviderName string
	PoolID       *uuid.UUID `gorm:"type:uuid;index"`
	ScaleSetID   *uint      `gorm:"index"`

	EntityType params.ForgeEntityType
	EntityID   string `gorm:"index"`
	EntityName string

	Flavor     string
	Image      string
	OSType     commonParams.OSType
	OSArch     commonParams.OSArch
	ExtraSpecs datatypes.JSON

	StartedAt *time.Time `gorm:"index"`
	EndedAt   *time.Time `gorm:"index"`
	EndReason params.UsageEndReason

	ProvisioningModel commonParams.ProvisioningModel `gorm:"index"`
	HourlyPrice       *float64
	Currency          string
	Price             datatypes.JSON
	PriceStatus       params.UsagePriceStatus `gorm:"index"`
	PriceError        string
	PricedAt          *time.Time

	JobID          int64
	JobName        string
	JobRepository  string
	WorkflowRunURL string
}

func (u *InstanceUsage) BeforeCreate(_ *gorm.DB) error {
	if u.ID == uuid.Nil {
		newID, err := uuid.NewRandom()
		if err != nil {
			return fmt.Errorf("error generating id: %w", err)
		}
		u.ID = newID
	}
	return nil
}

// usageSource is what an instance usage record copies from the pool or
// scale set that owns the instance.
type usageSource struct {
	PoolID       *uuid.UUID
	ScaleSetID   *uint
	ProviderName string
	Flavor       string
	Image        string
	OSType       commonParams.OSType
	OSArch       commonParams.OSArch
	ExtraSpecs   datatypes.JSON

	RepoID          *uuid.UUID
	OrgID           *uuid.UUID
	EnterpriseID    *uuid.UUID
	ForgeInstanceID *uuid.UUID
}

func poolUsageSource(pool Pool) usageSource {
	return usageSource{
		PoolID:          &pool.ID,
		ProviderName:    pool.ProviderName,
		Flavor:          pool.Flavor,
		Image:           pool.Image,
		OSType:          pool.OSType,
		OSArch:          pool.OSArch,
		ExtraSpecs:      pool.ExtraSpecs,
		RepoID:          pool.RepoID,
		OrgID:           pool.OrgID,
		EnterpriseID:    pool.EnterpriseID,
		ForgeInstanceID: pool.ForgeInstanceID,
	}
}

func scaleSetUsageSource(scaleSet ScaleSet) usageSource {
	return usageSource{
		ScaleSetID:   &scaleSet.ID,
		ProviderName: scaleSet.ProviderName,
		Flavor:       scaleSet.Flavor,
		Image:        scaleSet.Image,
		OSType:       scaleSet.OSType,
		OSArch:       scaleSet.OSArch,
		ExtraSpecs:   scaleSet.ExtraSpecs,
		RepoID:       scaleSet.RepoID,
		OrgID:        scaleSet.OrgID,
		EnterpriseID: scaleSet.EnterpriseID,
	}
}

// usageEntity returns the type, ID and display name of the entity that owns
// src. The name is copied so that it survives the entity.
func usageEntity(tx *gorm.DB, src usageSource) (params.ForgeEntityType, string, string, error) {
	switch {
	case src.RepoID != nil:
		var repo Repository
		if err := tx.Select("id", "owner", "name").Where("id = ?", *src.RepoID).First(&repo).Error; err != nil {
			return "", "", "", fmt.Errorf("error fetching repository: %w", err)
		}
		return params.ForgeEntityTypeRepository, repo.ID.String(), repo.Owner + "/" + repo.Name, nil
	case src.OrgID != nil:
		var org Organization
		if err := tx.Select("id", "name").Where("id = ?", *src.OrgID).First(&org).Error; err != nil {
			return "", "", "", fmt.Errorf("error fetching organization: %w", err)
		}
		return params.ForgeEntityTypeOrganization, org.ID.String(), org.Name, nil
	case src.EnterpriseID != nil:
		var ent Enterprise
		if err := tx.Select("id", "name").Where("id = ?", *src.EnterpriseID).First(&ent).Error; err != nil {
			return "", "", "", fmt.Errorf("error fetching enterprise: %w", err)
		}
		return params.ForgeEntityTypeEnterprise, ent.ID.String(), ent.Name, nil
	case src.ForgeInstanceID != nil:
		var fi ForgeInstance
		if err := tx.Select("id", "endpoint_name").Where("id = ?", *src.ForgeInstanceID).First(&fi).Error; err != nil {
			return "", "", "", fmt.Errorf("error fetching forge instance: %w", err)
		}
		name := ""
		if fi.EndpointName != nil {
			name = *fi.EndpointName
		}
		return params.ForgeEntityTypeInstance, fi.ID.String(), name, nil
	}
	return "", "", "", nil
}

// createInstanceUsage opens the usage record of a newly created instance.
func (s *sqlDatabase) createInstanceUsage(tx *gorm.DB, instance Instance, src usageSource) error {
	entityType, entityID, entityName, err := usageEntity(tx, src)
	if err != nil {
		return fmt.Errorf("error creating instance usage: %w", err)
	}
	// All usage times are UTC: SQLite compares times as text.
	now := time.Now().UTC()
	usage := InstanceUsage{
		CreatedAt:    now,
		UpdatedAt:    now,
		InstanceID:   instance.ID,
		InstanceName: instance.Name,
		ProviderName: src.ProviderName,
		PoolID:       src.PoolID,
		ScaleSetID:   src.ScaleSetID,
		EntityType:   entityType,
		EntityID:     entityID,
		EntityName:   entityName,
		Flavor:       src.Flavor,
		Image:        src.Image,
		OSType:       instance.OSType,
		OSArch:       instance.OSArch,
		ExtraSpecs:   src.ExtraSpecs,
		PriceStatus:  params.UsagePricePending,
	}
	if usage.OSType == "" {
		usage.OSType = src.OSType
	}
	if usage.OSArch == "" {
		usage.OSArch = src.OSArch
	}
	if err := tx.Create(&usage).Error; err != nil {
		return fmt.Errorf("error creating instance usage: %w", err)
	}
	return nil
}

func openUsage(tx *gorm.DB, instanceID uuid.UUID) *gorm.DB {
	return tx.Model(&InstanceUsage{}).Where("instance_id = ? AND ended_at IS NULL", instanceID)
}

// recordInstanceUsageUpdate follows an instance update: billing starts when
// the provider reports the instance as running, and stops once the provider
// has deleted it.
func (s *sqlDatabase) recordInstanceUsageUpdate(tx *gorm.DB, instance Instance, previous commonParams.InstanceStatus, now time.Time) error {
	if instance.ProviderID != nil && *instance.ProviderID != "" {
		if err := openUsage(tx, instance.ID).Where("provider_id <> ?", *instance.ProviderID).
			Update("provider_id", *instance.ProviderID).Error; err != nil {
			return fmt.Errorf("error updating instance usage: %w", err)
		}
	}
	if instance.Status == previous {
		return nil
	}
	switch instance.Status {
	case commonParams.InstanceRunning:
		if err := openUsage(tx, instance.ID).Where("started_at IS NULL").
			Update("started_at", now).Error; err != nil {
			return fmt.Errorf("error updating instance usage: %w", err)
		}
	case commonParams.InstanceDeleted:
		return s.closeInstanceUsage(tx, instance.ID, now, params.UsageEndDeleted)
	}
	return nil
}

func (s *sqlDatabase) closeInstanceUsage(tx *gorm.DB, instanceID uuid.UUID, now time.Time, reason params.UsageEndReason) error {
	if err := openUsage(tx, instanceID).Updates(map[string]any{
		"ended_at":   now,
		"end_reason": reason,
	}).Error; err != nil {
		return fmt.Errorf("error closing instance usage: %w", err)
	}
	return nil
}

// recordInstanceUsageJob copies the job that runs on an instance to its usage
// record. Jobs are deleted when they complete, so this is done as soon as
// the job is linked to the instance.
func (s *sqlDatabase) recordInstanceUsageJob(tx *gorm.DB, job WorkflowJob) error {
	if job.InstanceID == nil {
		return nil
	}
	repository := job.RepositoryName
	if job.RepositoryOwner != "" {
		repository = job.RepositoryOwner + "/" + job.RepositoryName
	}
	jobID := job.WorkflowJobID
	if jobID == 0 {
		jobID = job.ID
	}
	updates := map[string]any{
		"job_id":         jobID,
		"job_name":       job.Name,
		"job_repository": repository,
	}
	if job.WorkflowRunURL != "" {
		updates["workflow_run_url"] = job.WorkflowRunURL
	}
	if err := tx.Model(&InstanceUsage{}).Where("instance_id = ?", *job.InstanceID).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("error updating instance usage job: %w", err)
	}
	return nil
}

func (s *sqlDatabase) sqlToParamsInstanceUsage(u InstanceUsage) (params.InstanceUsage, error) {
	ret := params.InstanceUsage{
		ID:                u.ID.String(),
		InstanceID:        u.InstanceID.String(),
		InstanceName:      u.InstanceName,
		ProviderID:        u.ProviderID,
		ProviderName:      u.ProviderName,
		EntityType:        u.EntityType,
		EntityID:          u.EntityID,
		EntityName:        u.EntityName,
		Flavor:            u.Flavor,
		Image:             u.Image,
		OSType:            u.OSType,
		OSArch:            u.OSArch,
		CreatedAt:         u.CreatedAt,
		StartedAt:         u.StartedAt,
		EndedAt:           u.EndedAt,
		EndReason:         u.EndReason,
		ProvisioningModel: u.ProvisioningModel,
		HourlyPrice:       u.HourlyPrice,
		Currency:          u.Currency,
		PriceStatus:       u.PriceStatus,
		PriceError:        u.PriceError,
		PricedAt:          u.PricedAt,
		JobID:             u.JobID,
		JobName:           u.JobName,
		JobRepository:     u.JobRepository,
		WorkflowRunURL:    u.WorkflowRunURL,
	}
	if len(u.ExtraSpecs) > 0 {
		ret.ExtraSpecs = json.RawMessage(u.ExtraSpecs)
	}
	if u.PoolID != nil {
		ret.PoolID = u.PoolID.String()
	}
	if u.ScaleSetID != nil {
		ret.ScaleSetID = *u.ScaleSetID
	}
	if len(u.Price) > 0 {
		var price commonParams.InstancePrice
		if err := json.Unmarshal(u.Price, &price); err != nil {
			return params.InstanceUsage{}, fmt.Errorf("error decoding instance price: %w", err)
		}
		ret.Price = &price
	}
	return ret, nil
}

func (s *sqlDatabase) instanceUsageQuery(filter params.InstanceUsageFilter) (*gorm.DB, error) {
	q := s.conn.Model(&InstanceUsage{})
	if !filter.End.IsZero() {
		q = q.Where("COALESCE(started_at, created_at) < ?", filter.End)
	}
	if !filter.Start.IsZero() {
		q = q.Where("ended_at IS NULL OR ended_at > ?", filter.Start)
	}
	if filter.PoolID != "" {
		poolID, err := uuid.Parse(filter.PoolID)
		if err != nil {
			return nil, runnerErrors.NewBadRequestError("invalid pool ID %q", filter.PoolID)
		}
		q = q.Where("pool_id = ?", poolID)
	}
	if filter.ScaleSetID != 0 {
		q = q.Where("scale_set_id = ?", filter.ScaleSetID)
	}
	if filter.EntityID != "" {
		q = q.Where("entity_id = ?", filter.EntityID)
	}
	if filter.InstanceName != "" {
		q = q.Where("instance_name = ?", filter.InstanceName)
	}
	if filter.ProviderName != "" {
		q = q.Where("provider_name = ?", filter.ProviderName)
	}
	if filter.ProvisioningModel != "" {
		q = q.Where("provisioning_model = ?", filter.ProvisioningModel)
	}
	if filter.RunningOnly {
		q = q.Where("ended_at IS NULL")
	}
	return q.Order("COALESCE(started_at, created_at) DESC").Order("id"), nil
}

func (s *sqlDatabase) ListInstanceUsage(_ context.Context, filter params.InstanceUsageFilter, page, pageSize uint64) (params.InstanceUsagePaginatedResponse, error) {
	if page == 0 {
		page = 1
	}
	if pageSize == 0 {
		pageSize = 50
	}
	q, err := s.instanceUsageQuery(filter)
	if err != nil {
		return params.InstanceUsagePaginatedResponse{}, err
	}

	var total int64
	if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return params.InstanceUsagePaginatedResponse{}, fmt.Errorf("error counting instance usage: %w", err)
	}
	totalPages := (uint64(total) + pageSize - 1) / pageSize
	offset := (page - 1) * pageSize
	if offset > math.MaxInt || pageSize > math.MaxInt {
		return params.InstanceUsagePaginatedResponse{}, runnerErrors.NewBadRequestError("page out of range")
	}

	var rows []InstanceUsage
	if err := q.Limit(int(pageSize)).Offset(int(offset)).Find(&rows).Error; err != nil {
		return params.InstanceUsagePaginatedResponse{}, fmt.Errorf("error listing instance usage: %w", err)
	}
	results := make([]params.InstanceUsage, 0, len(rows))
	for _, row := range rows {
		usage, err := s.sqlToParamsInstanceUsage(row)
		if err != nil {
			return params.InstanceUsagePaginatedResponse{}, err
		}
		results = append(results, usage)
	}

	ret := params.InstanceUsagePaginatedResponse{
		TotalCount:  uint64(total),
		Pages:       totalPages,
		CurrentPage: page,
		Results:     results,
	}
	if page < totalPages {
		next := page + 1
		ret.NextPage = &next
	}
	if page > 1 {
		prev := page - 1
		ret.PreviousPage = &prev
	}
	return ret, nil
}

func (s *sqlDatabase) ListAllInstanceUsage(_ context.Context, filter params.InstanceUsageFilter) ([]params.InstanceUsage, error) {
	q, err := s.instanceUsageQuery(filter)
	if err != nil {
		return nil, err
	}
	var results []params.InstanceUsage
	var rows []InstanceUsage
	err = q.FindInBatches(&rows, 1000, func(_ *gorm.DB, _ int) error {
		for _, row := range rows {
			usage, err := s.sqlToParamsInstanceUsage(row)
			if err != nil {
				return err
			}
			results = append(results, usage)
		}
		return nil
	}).Error
	if err != nil {
		return nil, fmt.Errorf("error listing instance usage: %w", err)
	}
	return results, nil
}

func (s *sqlDatabase) ListInstanceUsagePendingPrice(_ context.Context, retryErrorsBefore time.Time, limit int) ([]params.InstanceUsage, error) {
	var rows []InstanceUsage
	q := s.conn.Model(&InstanceUsage{}).
		Where("price_status = ? OR (price_status = ? AND (priced_at IS NULL OR priced_at < ?))",
			params.UsagePricePending, params.UsagePriceError, retryErrorsBefore).
		Order("created_at").
		Limit(limit).
		Find(&rows)
	if q.Error != nil {
		return nil, fmt.Errorf("error listing unpriced instance usage: %w", q.Error)
	}
	results := make([]params.InstanceUsage, 0, len(rows))
	for _, row := range rows {
		usage, err := s.sqlToParamsInstanceUsage(row)
		if err != nil {
			return nil, err
		}
		results = append(results, usage)
	}
	return results, nil
}

func (s *sqlDatabase) SetInstanceUsagePrice(_ context.Context, usageID string, param params.SetInstanceUsagePriceParams) error {
	id, err := uuid.Parse(usageID)
	if err != nil {
		return runnerErrors.NewBadRequestError("invalid usage ID %q", usageID)
	}
	now := time.Now().UTC()
	updates := map[string]any{
		"price_status": param.Status,
		"price_error":  param.Error,
		"priced_at":    now,
	}
	if param.Price != nil {
		asJs, err := json.Marshal(param.Price)
		if err != nil {
			return fmt.Errorf("error encoding instance price: %w", err)
		}
		updates["price"] = datatypes.JSON(asJs)
		updates["hourly_price"] = param.Price.HourlyPrice
		updates["currency"] = param.Price.Currency
		updates["provisioning_model"] = param.Price.ProvisioningModel
	}
	q := s.conn.Model(&InstanceUsage{}).Where("id = ?", id).Updates(updates)
	if q.Error != nil {
		return fmt.Errorf("error setting instance usage price: %w", q.Error)
	}
	if q.RowsAffected == 0 {
		return runnerErrors.ErrNotFound
	}
	return nil
}

// CloseOrphanedInstanceUsage closes open usage records whose instance no
// longer exists. Usage records are maintained in the instance transactions,
// so this only catches instances removed by other means (e.g. manual
// database edits).
func (s *sqlDatabase) CloseOrphanedInstanceUsage(_ context.Context) (int64, error) {
	q := s.conn.Model(&InstanceUsage{}).
		Where("ended_at IS NULL AND instance_id NOT IN (?)", s.conn.Model(&Instance{}).Select("id")).
		Updates(map[string]any{
			"ended_at":   time.Now().UTC(),
			"end_reason": params.UsageEndOrphaned,
		})
	if q.Error != nil {
		return 0, fmt.Errorf("error closing orphaned instance usage: %w", q.Error)
	}
	return q.RowsAffected, nil
}

// getInstanceUsageByInstance is used by tests.
func (s *sqlDatabase) getInstanceUsageByInstance(instanceID uuid.UUID) (InstanceUsage, error) {
	var usage InstanceUsage
	if err := s.conn.Where("instance_id = ?", instanceID).First(&usage).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return InstanceUsage{}, runnerErrors.ErrNotFound
		}
		return InstanceUsage{}, err
	}
	return usage, nil
}
