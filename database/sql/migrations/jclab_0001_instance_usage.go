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

package migrations

import (
	"fmt"
	"time"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Migrations of the jclab fork use the "jclab_" prefix, which sorts after
// the numbered upstream migrations, so that upstream can keep numbering its
// own without collisions.

// instanceUsageJclab0001 is a frozen copy of the InstanceUsage model.
type instanceUsageJclab0001 struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key;"`
	CreatedAt time.Time
	UpdatedAt time.Time

	InstanceID   uuid.UUID `gorm:"type:uuid;index"`
	InstanceName string    `gorm:"index"`
	ProviderID   string
	ProviderName string
	PoolID       *uuid.UUID `gorm:"type:uuid;index"`
	ScaleSetID   *uint      `gorm:"index"`

	EntityType string
	EntityID   string `gorm:"index"`
	EntityName string

	Flavor     string
	Image      string
	OSType     string
	OSArch     string
	ExtraSpecs datatypes.JSON

	StartedAt *time.Time `gorm:"index"`
	EndedAt   *time.Time `gorm:"index"`
	EndReason string

	ProvisioningModel string `gorm:"index"`
	HourlyPrice       *float64
	Currency          string
	Price             datatypes.JSON
	PriceStatus       string `gorm:"index"`
	PriceError        string
	PricedAt          *time.Time

	JobID          int64
	JobName        string
	JobRepository  string
	WorkflowRunURL string
}

func (instanceUsageJclab0001) TableName() string { return "instance_usages" }

// The stubs below read the existing instances to backfill their usage.

type instanceJclab0001 struct {
	ID           uuid.UUID
	CreatedAt    time.Time
	Name         string
	ProviderID   *string
	OSType       string
	OSArch       string
	Status       string
	PoolID       *uuid.UUID
	ScaleSetFkID *uint
}

func (instanceJclab0001) TableName() string { return "instances" }

type usageSourceJclab0001 struct {
	ProviderName    string
	Flavor          string
	Image           string
	ExtraSpecs      datatypes.JSON
	RepoID          *uuid.UUID
	OrgID           *uuid.UUID
	EnterpriseID    *uuid.UUID
	ForgeInstanceID *uuid.UUID
}

// billedStatusesJclab0001 are the statuses of instances that exist in the
// provider. Their billing is assumed to have started when they were created.
var billedStatusesJclab0001 = map[string]bool{
	"running":              true,
	"stopped":              true,
	"error":                true,
	"pending_delete":       true,
	"pending_force_delete": true,
	"deleting":             true,
}

func backfillInstanceUsageJclab0001(tx *gorm.DB) error {
	var instances []instanceJclab0001
	if err := tx.Find(&instances).Error; err != nil {
		return fmt.Errorf("listing instances: %w", err)
	}
	now := time.Now().UTC()
	for _, instance := range instances {
		var src usageSourceJclab0001
		switch {
		case instance.PoolID != nil:
			if err := tx.Table("pools").Select("provider_name, flavor, image, extra_specs, repo_id, org_id, enterprise_id, forge_instance_id").
				Where("id = ?", *instance.PoolID).Take(&src).Error; err != nil {
				return fmt.Errorf("fetching pool of instance %s: %w", instance.Name, err)
			}
		case instance.ScaleSetFkID != nil:
			if err := tx.Table("scale_sets").Select("provider_name, flavor, image, extra_specs, repo_id, org_id, enterprise_id").
				Where("id = ?", *instance.ScaleSetFkID).Take(&src).Error; err != nil {
				return fmt.Errorf("fetching scale set of instance %s: %w", instance.Name, err)
			}
		}

		entityType, entityID, entityName, err := entityJclab0001(tx, src)
		if err != nil {
			return fmt.Errorf("fetching entity of instance %s: %w", instance.Name, err)
		}

		usage := instanceUsageJclab0001{
			ID:           uuid.New(),
			CreatedAt:    now,
			UpdatedAt:    now,
			InstanceID:   instance.ID,
			InstanceName: instance.Name,
			ProviderName: src.ProviderName,
			PoolID:       instance.PoolID,
			ScaleSetID:   instance.ScaleSetFkID,
			EntityType:   entityType,
			EntityID:     entityID,
			EntityName:   entityName,
			Flavor:       src.Flavor,
			Image:        src.Image,
			OSType:       instance.OSType,
			OSArch:       instance.OSArch,
			ExtraSpecs:   src.ExtraSpecs,
			PriceStatus:  "pending",
		}
		if instance.ProviderID != nil {
			usage.ProviderID = *instance.ProviderID
		}
		if billedStatusesJclab0001[instance.Status] {
			started := instance.CreatedAt.UTC()
			usage.StartedAt = &started
		}
		if err := tx.Create(&usage).Error; err != nil {
			return fmt.Errorf("creating usage of instance %s: %w", instance.Name, err)
		}
	}
	return nil
}

func entityJclab0001(tx *gorm.DB, src usageSourceJclab0001) (string, string, string, error) {
	var row struct {
		Owner        string
		Name         string
		EndpointName *string
	}
	switch {
	case src.RepoID != nil:
		err := tx.Table("repositories").Select("owner, name").Where("id = ?", *src.RepoID).Take(&row).Error
		return "repository", src.RepoID.String(), row.Owner + "/" + row.Name, err
	case src.OrgID != nil:
		err := tx.Table("organizations").Select("name").Where("id = ?", *src.OrgID).Take(&row).Error
		return "organization", src.OrgID.String(), row.Name, err
	case src.EnterpriseID != nil:
		err := tx.Table("enterprises").Select("name").Where("id = ?", *src.EnterpriseID).Take(&row).Error
		return "enterprise", src.EnterpriseID.String(), row.Name, err
	case src.ForgeInstanceID != nil:
		err := tx.Table("forge_instances").Select("endpoint_name").Where("id = ?", *src.ForgeInstanceID).Take(&row).Error
		name := ""
		if row.EndpointName != nil {
			name = *row.EndpointName
		}
		return "instance", src.ForgeInstanceID.String(), name, err
	}
	return "", "", "", nil
}

func init() {
	Register(&gormigrate.Migration{
		ID: "jclab_0001_instance_usage",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.AutoMigrate(&instanceUsageJclab0001{}); err != nil {
				return err
			}
			return backfillInstanceUsageJclab0001(tx)
		},
	})
}
