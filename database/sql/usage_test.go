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
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"

	commonParams "github.com/cloudbase/garm-provider-common/params"
	dbCommon "github.com/cloudbase/garm/database/common"
	"github.com/cloudbase/garm/database/watcher"
	garmTesting "github.com/cloudbase/garm/internal/testing"
	"github.com/cloudbase/garm/params"
)

type UsageTestSuite struct {
	suite.Suite
	Store    dbCommon.Store
	adminCtx context.Context
	org      params.Organization
	pool     params.Pool
}

func (s *UsageTestSuite) TearDownTest() {
	watcher.CloseWatcher()
}

func (s *UsageTestSuite) SetupTest() {
	ctx := context.Background()
	watcher.InitWatcher(ctx)
	db := newTestDB(s.T())
	s.Store = db
	s.adminCtx = garmTesting.ImpersonateAdminContext(ctx, db, s.T())

	githubEndpoint := garmTesting.CreateDefaultGithubEndpoint(s.adminCtx, db, s.T())
	creds := garmTesting.CreateTestGithubCredentials(s.adminCtx, "new-creds", db, s.T(), githubEndpoint)
	org, err := db.CreateOrganization(s.adminCtx, "test-org", creds, "test-webhookSecret", params.PoolBalancerTypeRoundRobin, false)
	s.Require().NoError(err)
	s.org = org

	entity, err := org.GetEntity()
	s.Require().NoError(err)
	s.pool, err = db.CreateEntityPool(s.adminCtx, entity, params.CreatePoolParams{
		ProviderName:   "gcp",
		MaxRunners:     10,
		MinIdleRunners: 0,
		Image:          "test-image",
		Flavor:         "c4d-standard-8-lssd",
		OSType:         "linux",
		OSArch:         "amd64",
		Tags:           []string{"linux"},
		ExtraSpecs:     json.RawMessage(`{"spot": true}`),
	})
	s.Require().NoError(err)
}

func (s *UsageTestSuite) createInstance(name string) params.Instance {
	instance, err := s.Store.CreateInstance(s.adminCtx, s.pool.ID, params.CreateInstanceParams{
		Name:         name,
		OSType:       commonParams.Linux,
		OSArch:       commonParams.Amd64,
		Status:       commonParams.InstancePendingCreate,
		RunnerStatus: params.RunnerPending,
	})
	s.Require().NoError(err)
	return instance
}

func (s *UsageTestSuite) usageOf(instance params.Instance) InstanceUsage {
	id, err := uuid.Parse(instance.ID)
	s.Require().NoError(err)
	usage, err := s.Store.(*sqlDatabase).getInstanceUsageByInstance(id)
	s.Require().NoError(err)
	return usage
}

func (s *UsageTestSuite) setStatus(instance params.Instance, status commonParams.InstanceStatus) {
	_, err := s.Store.UpdateInstance(s.adminCtx, instance.Name, params.UpdateInstanceParams{Status: status})
	s.Require().NoError(err)
}

// run moves a new instance to running, as the provider worker does.
func (s *UsageTestSuite) run(instance params.Instance) {
	s.setStatus(instance, commonParams.InstanceCreating)
	s.setStatus(instance, commonParams.InstanceRunning)
}

func (s *UsageTestSuite) TestCreateInstanceOpensUsage() {
	instance := s.createInstance("runner-1")

	usage := s.usageOf(instance)
	s.Require().Equal(instance.Name, usage.InstanceName)
	s.Require().Equal("gcp", usage.ProviderName)
	s.Require().Equal("c4d-standard-8-lssd", usage.Flavor)
	s.Require().Equal("test-image", usage.Image)
	s.Require().Equal(s.pool.ID, usage.PoolID.String())
	s.Require().Equal(params.ForgeEntityTypeOrganization, usage.EntityType)
	s.Require().Equal(s.org.ID, usage.EntityID)
	s.Require().Equal("test-org", usage.EntityName)
	s.Require().JSONEq(`{"spot": true}`, string(usage.ExtraSpecs))
	s.Require().Equal(params.UsagePricePending, usage.PriceStatus)
	s.Require().Nil(usage.StartedAt)
	s.Require().Nil(usage.EndedAt)
}

func (s *UsageTestSuite) TestLifecycle() {
	instance := s.createInstance("runner-1")

	s.setStatus(instance, commonParams.InstanceCreating)
	s.Require().Nil(s.usageOf(instance).StartedAt, "billing starts when the instance runs")

	_, err := s.Store.UpdateInstance(s.adminCtx, instance.Name, params.UpdateInstanceParams{
		Status:     commonParams.InstanceRunning,
		ProviderID: "projects/p/zones/us-east4-a/instances/runner-1",
	})
	s.Require().NoError(err)
	usage := s.usageOf(instance)
	s.Require().NotNil(usage.StartedAt)
	s.Require().Equal("projects/p/zones/us-east4-a/instances/runner-1", usage.ProviderID)
	started := *usage.StartedAt

	// Later updates don't move the start.
	_, err = s.Store.UpdateInstance(s.adminCtx, instance.Name, params.UpdateInstanceParams{OSName: "ubuntu"})
	s.Require().NoError(err)
	s.Require().True(started.Equal(*s.usageOf(instance).StartedAt))

	s.setStatus(instance, commonParams.InstancePendingDelete)
	s.Require().Nil(s.usageOf(instance).EndedAt, "billing continues until the provider deletes the instance")
	s.setStatus(instance, commonParams.InstanceDeleting)
	s.setStatus(instance, commonParams.InstanceDeleted)
	usage = s.usageOf(instance)
	s.Require().NotNil(usage.EndedAt)
	s.Require().Equal(params.UsageEndDeleted, usage.EndReason)
	ended := *usage.EndedAt

	// Removing the row keeps the usage record and its end time.
	s.Require().NoError(s.Store.DeleteInstance(s.adminCtx, s.pool.ID, instance.Name))
	usage = s.usageOf(instance)
	s.Require().True(ended.Equal(*usage.EndedAt))
}

func (s *UsageTestSuite) TestDeleteInstanceClosesUsage() {
	instance := s.createInstance("runner-1")
	s.run(instance)

	s.Require().NoError(s.Store.DeleteInstanceByName(s.adminCtx, instance.Name))
	usage := s.usageOf(instance)
	s.Require().NotNil(usage.EndedAt)
	s.Require().Equal(params.UsageEndDeleted, usage.EndReason)
}

func (s *UsageTestSuite) TestJobIsRecorded() {
	instance := s.createInstance("runner-1")
	s.run(instance)

	_, err := s.Store.CreateOrUpdateJob(s.adminCtx, params.Job{
		WorkflowJobID:   4242,
		Name:            "build",
		Status:          string(params.JobStatusQueued),
		RepositoryOwner: "jclab",
		RepositoryName:  "app",
		WorkflowRunURL:  "https://github.com/jclab/app/actions/runs/1",
	})
	s.Require().NoError(err)
	s.Require().Zero(s.usageOf(instance).JobID, "the job is not linked to the runner yet")

	started := time.Date(2026, 10, 2, 1, 0, 0, 0, time.UTC)
	_, err = s.Store.CreateOrUpdateJob(s.adminCtx, params.Job{
		WorkflowJobID:  4242,
		Status:         string(params.JobStatusInProgress),
		RunnerName:     instance.Name,
		StartedAt:      started,
		WorkflowRunURL: "https://github.com/jclab/app/actions/runs/1",
		JobURL:         "https://github.com/jclab/app/actions/runs/1/job/4242",
		RunAttempt:     2,
		WorkflowName:   "CI",
		HeadBranch:     "main",
	})
	s.Require().NoError(err)
	usage := s.usageOf(instance)
	s.Require().Equal(int64(4242), usage.JobID)
	s.Require().Equal("build", usage.JobName)
	s.Require().Equal("jclab/app", usage.JobRepository)
	s.Require().Equal("https://github.com/jclab/app/actions/runs/1", usage.WorkflowRunURL)
	s.Require().Equal("https://github.com/jclab/app/actions/runs/1/job/4242", usage.JobURL)
	s.Require().Equal(int64(2), usage.RunAttempt)
	s.Require().Equal("CI", usage.WorkflowName)
	s.Require().Equal("main", usage.HeadBranch)
	s.Require().True(started.Equal(*usage.JobStartedAt))
	s.Require().Empty(usage.JobConclusion)
	s.Require().Nil(usage.JobCompletedAt)

	// The completed update brings the conclusion. Fields it does not carry
	// are kept.
	completed := started.Add(10 * time.Minute)
	_, err = s.Store.CreateOrUpdateJob(s.adminCtx, params.Job{
		WorkflowJobID: 4242,
		Status:        string(params.JobStatusCompleted),
		Conclusion:    "success",
		RunnerName:    instance.Name,
		StartedAt:     started,
		CompletedAt:   completed,
	})
	s.Require().NoError(err)
	usage = s.usageOf(instance)
	s.Require().Equal("success", usage.JobConclusion)
	s.Require().True(completed.Equal(*usage.JobCompletedAt))
	s.Require().Equal("CI", usage.WorkflowName)
	s.Require().Equal("https://github.com/jclab/app/actions/runs/1/job/4242", usage.JobURL)

	all, err := s.Store.ListAllInstanceUsage(s.adminCtx, params.InstanceUsageFilter{InstanceName: instance.Name})
	s.Require().NoError(err)
	s.Require().Len(all, 1)
	s.Require().Equal("CI", all[0].WorkflowName)
	s.Require().Equal("success", all[0].JobConclusion)
	s.Require().NotNil(all[0].JobCompletedAt)
}

func (s *UsageTestSuite) TestScaleSetInstanceOpensUsage() {
	entity, err := s.org.GetEntity()
	s.Require().NoError(err)
	scaleSet, err := s.Store.CreateEntityScaleSet(s.adminCtx, entity, params.CreateScaleSetParams{
		Name:         "test-scaleset",
		ProviderName: "gcp",
		MaxRunners:   10,
		Image:        "ss-image",
		Flavor:       "c4a-standard-8-lssd",
		OSType:       commonParams.Linux,
		OSArch:       commonParams.Arm64,
	})
	s.Require().NoError(err)
	instance, err := s.Store.CreateScaleSetInstance(s.adminCtx, scaleSet.ID, params.CreateInstanceParams{
		Name:   "ss-runner-1",
		OSType: commonParams.Linux,
		OSArch: commonParams.Arm64,
		Status: commonParams.InstancePendingCreate,
	})
	s.Require().NoError(err)

	usage := s.usageOf(instance)
	s.Require().NotNil(usage.ScaleSetID)
	s.Require().Equal(scaleSet.ID, *usage.ScaleSetID)
	s.Require().Nil(usage.PoolID)
	s.Require().Equal("c4a-standard-8-lssd", usage.Flavor)
	s.Require().Equal("test-org", usage.EntityName)
}

// setTimes rewrites the times of the usage record of instance.
func (s *UsageTestSuite) setTimes(instance params.Instance, started, ended *time.Time) {
	updates := map[string]any{"started_at": started, "ended_at": ended}
	if started != nil {
		updates["created_at"] = started.Add(-time.Minute)
	}
	s.Require().NoError(s.Store.(*sqlDatabase).conn.Model(&InstanceUsage{}).
		Where("instance_name = ?", instance.Name).Updates(updates).Error)
}

func ptr[T any](v T) *T { return &v }

func (s *UsageTestSuite) TestListInstanceUsageDateRange() {
	day := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	before := s.createInstance("before")
	s.setTimes(before, ptr(day.Add(-3*time.Hour)), ptr(day.Add(-time.Hour)))
	across := s.createInstance("across")
	s.setTimes(across, ptr(day.Add(-time.Hour)), ptr(day.Add(time.Hour)))
	inside := s.createInstance("inside")
	s.setTimes(inside, ptr(day.Add(2*time.Hour)), ptr(day.Add(3*time.Hour)))
	running := s.createInstance("running")
	s.setTimes(running, ptr(day.Add(5*time.Hour)), nil)
	after := s.createInstance("after")
	s.setTimes(after, ptr(day.Add(25*time.Hour)), nil)

	filter := params.InstanceUsageFilter{Start: day, End: day.Add(24 * time.Hour)}
	ret, err := s.Store.ListInstanceUsage(s.adminCtx, filter, 1, 2)
	s.Require().NoError(err)
	s.Require().Equal(uint64(3), ret.TotalCount)
	s.Require().Equal(uint64(2), ret.Pages)
	s.Require().NotNil(ret.NextPage)
	s.Require().Equal([]string{"running", "inside"}, usageNames(ret.Results))

	ret, err = s.Store.ListInstanceUsage(s.adminCtx, filter, 2, 2)
	s.Require().NoError(err)
	s.Require().Equal([]string{"across"}, usageNames(ret.Results))
	s.Require().NotNil(ret.PreviousPage)

	all, err := s.Store.ListAllInstanceUsage(s.adminCtx, filter)
	s.Require().NoError(err)
	s.Require().Equal([]string{"running", "inside", "across"}, usageNames(all))

	all, err = s.Store.ListAllInstanceUsage(s.adminCtx, params.InstanceUsageFilter{RunningOnly: true})
	s.Require().NoError(err)
	s.Require().Equal([]string{"after", "running"}, usageNames(all))

	all, err = s.Store.ListAllInstanceUsage(s.adminCtx, params.InstanceUsageFilter{InstanceName: "inside", PoolID: s.pool.ID})
	s.Require().NoError(err)
	s.Require().Equal([]string{"inside"}, usageNames(all))

	_, err = s.Store.ListAllInstanceUsage(s.adminCtx, params.InstanceUsageFilter{PoolID: "not-a-uuid"})
	s.Require().Error(err)
}

func usageNames(usage []params.InstanceUsage) []string {
	names := make([]string, 0, len(usage))
	for _, u := range usage {
		names = append(names, u.InstanceName)
	}
	return names
}

func (s *UsageTestSuite) TestPricing() {
	first := s.createInstance("runner-1")
	second := s.createInstance("runner-2")

	pending, err := s.Store.ListInstanceUsagePendingPrice(s.adminCtx, time.Now().UTC(), 10)
	s.Require().NoError(err)
	s.Require().Equal([]string{"runner-1", "runner-2"}, usageNames(pending))
	s.Require().JSONEq(`{"spot": true}`, string(pending[0].ExtraSpecs))

	price := &commonParams.InstancePrice{
		Currency:          "USD",
		HourlyPrice:       0.25,
		ProvisioningModel: commonParams.ProvisioningModelSpot,
		Components:        []commonParams.InstancePriceComponent{{Name: "vcpu", Quantity: 8, HourlyPrice: 0.25}},
	}
	s.Require().NoError(s.Store.SetInstanceUsagePrice(s.adminCtx, pending[0].ID, params.SetInstanceUsagePriceParams{
		Status: params.UsagePriceOK,
		Price:  price,
	}))
	s.Require().NoError(s.Store.SetInstanceUsagePrice(s.adminCtx, pending[1].ID, params.SetInstanceUsagePriceParams{
		Status: params.UsagePriceError,
		Error:  "boom",
	}))

	all, err := s.Store.ListAllInstanceUsage(s.adminCtx, params.InstanceUsageFilter{InstanceName: first.Name})
	s.Require().NoError(err)
	s.Require().Len(all, 1)
	s.Require().Equal(params.UsagePriceOK, all[0].PriceStatus)
	s.Require().Equal(0.25, *all[0].HourlyPrice)
	s.Require().Equal("USD", all[0].Currency)
	s.Require().Equal(commonParams.ProvisioningModelSpot, all[0].ProvisioningModel)
	s.Require().Equal(price, all[0].Price)

	spot, err := s.Store.ListAllInstanceUsage(s.adminCtx, params.InstanceUsageFilter{ProvisioningModel: commonParams.ProvisioningModelSpot})
	s.Require().NoError(err)
	s.Require().Equal([]string{first.Name}, usageNames(spot))

	// Failed lookups are retried once they are older than retryErrorsBefore.
	pending, err = s.Store.ListInstanceUsagePendingPrice(s.adminCtx, time.Now().UTC().Add(-time.Minute), 10)
	s.Require().NoError(err)
	s.Require().Empty(pending)
	pending, err = s.Store.ListInstanceUsagePendingPrice(s.adminCtx, time.Now().UTC().Add(time.Minute), 10)
	s.Require().NoError(err)
	s.Require().Equal([]string{second.Name}, usageNames(pending))
	s.Require().Equal("boom", pending[0].PriceError)

	s.Require().ErrorContains(s.Store.SetInstanceUsagePrice(s.adminCtx, uuid.NewString(), params.SetInstanceUsagePriceParams{Status: params.UsagePriceOK}), "not found")
}

func (s *UsageTestSuite) TestCloseOrphanedInstanceUsage() {
	kept := s.createInstance("kept")
	orphan := s.createInstance("orphan")
	s.run(orphan)
	// Remove the instance without going through the store.
	s.Require().NoError(s.Store.(*sqlDatabase).conn.Unscoped().Where("name = ?", orphan.Name).Delete(&Instance{}).Error)

	closed, err := s.Store.CloseOrphanedInstanceUsage(s.adminCtx)
	s.Require().NoError(err)
	s.Require().Equal(int64(1), closed)
	s.Require().Equal(params.UsageEndOrphaned, s.usageOf(orphan).EndReason)
	s.Require().Nil(s.usageOf(kept).EndedAt)
}

func TestUsageTestSuite(t *testing.T) {
	suite.Run(t, new(UsageTestSuite))
}

// TestUsageMigrationBackfill upgrades a v0.2.1 database and checks that its
// instances get usage records.
func TestUsageMigrationBackfill(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	watcher.InitWatcher(ctx)
	defer watcher.CloseWatcher()

	cfg := garmTesting.GetTestSqliteDBConfig(t)
	execSQLFileOnFile(t, cfg.SQLite.DBFile, "migrations/testdata/v021_schema.sql")
	seedV021Data(t, cfg.SQLite.DBFile, cfg.Passphrase)
	blobCfg, err := cfg.SQLiteBlobDatabaseConfig()
	if err != nil {
		t.Fatal(err)
	}
	execSQLFileOnFile(t, blobCfg.SQLite.DBFile, "migrations/testdata/v021_blob_schema.sql")

	store, err := NewSQLStore(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	conn := store.(*sqlDatabase).conn

	var instances []Instance
	if err := conn.Find(&instances).Error; err != nil {
		t.Fatal(err)
	}
	var usage []InstanceUsage
	if err := conn.Find(&usage).Error; err != nil {
		t.Fatal(err)
	}
	if len(instances) == 0 {
		t.Skip("the v0.2.1 seed has no instances")
	}
	if len(usage) != len(instances) {
		t.Fatalf("expected %d usage records, got %d", len(instances), len(usage))
	}
	byInstance := map[uuid.UUID]InstanceUsage{}
	for _, u := range usage {
		byInstance[u.InstanceID] = u
	}
	for _, instance := range instances {
		u, ok := byInstance[instance.ID]
		if !ok {
			t.Fatalf("no usage record for instance %s", instance.Name)
		}
		if u.InstanceName != instance.Name || u.PriceStatus != params.UsagePricePending || u.EndedAt != nil {
			t.Fatalf("unexpected usage record for %s: %s", instance.Name, fmt.Sprintf("%+v", u))
		}
		if instance.Status == commonParams.InstanceRunning && u.StartedAt == nil {
			t.Fatalf("running instance %s has no start time", instance.Name)
		}
	}
}
