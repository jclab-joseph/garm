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

package pool

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/cloudbase/garm/params"
)

func TestParamsWorkflowJobToParamsJobRunDetails(t *testing.T) {
	mgr := &basePoolManager{entity: params.ForgeEntity{ID: uuid.NewString(), EntityType: params.ForgeEntityTypeRepository}}
	var hook params.WorkflowJob
	require.NoError(t, json.Unmarshal([]byte(`{
		"action": "in_progress",
		"workflow_job": {
			"id": 4242, "run_id": 77, "run_attempt": 3, "name": "build",
			"html_url": "https://github.com/jclab/app/actions/runs/77/job/4242",
			"workflow_name": "CI", "head_branch": "main",
			"runner_name": "garm-abc"
		},
		"repository": {"name": "app", "html_url": "https://github.com/jclab/app", "owner": {"login": "jclab"}}
	}`), &hook))

	job, err := mgr.paramsWorkflowJobToParamsJob(hook)
	require.NoError(t, err)
	require.Equal(t, "https://github.com/jclab/app/actions/runs/77", job.WorkflowRunURL)
	require.Equal(t, "https://github.com/jclab/app/actions/runs/77/job/4242", job.JobURL)
	require.Equal(t, int64(3), job.RunAttempt)
	require.Equal(t, "CI", job.WorkflowName)
	require.Equal(t, "main", job.HeadBranch)
	require.Equal(t, "garm-abc", job.RunnerName)
}
