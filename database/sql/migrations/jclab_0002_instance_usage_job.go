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
	"gorm.io/gorm"
)

// instanceUsageJclab0002 declares the job details added to instance usage
// records: the job web URL, run attempt, workflow, branch and outcome.
type instanceUsageJclab0002 struct {
	JobURL         string
	RunAttempt     int64
	WorkflowName   string
	HeadBranch     string
	JobConclusion  string
	JobStartedAt   *time.Time
	JobCompletedAt *time.Time
}

func (instanceUsageJclab0002) TableName() string { return "instance_usages" }

func init() {
	Register(&gormigrate.Migration{
		ID: "jclab_0002_instance_usage_job",
		Migrate: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&instanceUsageJclab0002{})
		},
	})
}
