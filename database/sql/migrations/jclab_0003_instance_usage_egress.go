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
	"time"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// instanceUsageJclab0003 declares the egress measurement of instance usage
// records.
type instanceUsageJclab0003 struct {
	EgressStatus        string `gorm:"index"`
	EgressError         string
	EgressFinal         bool `gorm:"index"`
	EgressCheckedAt     *time.Time
	EgressBytes         int64
	EgressExternalBytes int64
	EgressSentBytes     int64
	EgressCost          *float64
	EgressCurrency      string
	Egress              datatypes.JSON
}

func (instanceUsageJclab0003) TableName() string { return "instance_usages" }

func init() {
	Register(&gormigrate.Migration{
		ID: "jclab_0003_instance_usage_egress",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.AutoMigrate(&instanceUsageJclab0003{}); err != nil {
				return err
			}
			// Existing runs are measured too, as far as the metrics
			// retention allows. New columns are NULL in existing rows.
			return tx.Table("instance_usages").Where("egress_status = '' OR egress_status IS NULL").
				Updates(map[string]any{"egress_status": "pending", "egress_final": false}).Error
		},
	})
}
