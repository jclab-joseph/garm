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

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScaleSetJobWorkflowRef(t *testing.T) {
	job := ScaleSetJobMessage{JobWorkflowRef: "jclab/app/.github/workflows/ci.yml@refs/heads/feature/x"}.ToJob()
	require.Equal(t, ".github/workflows/ci.yml", job.WorkflowName)
	require.Equal(t, "feature/x", job.HeadBranch)

	job = ScaleSetJobMessage{JobWorkflowRef: "jclab/app/.github/workflows/release.yml@refs/tags/v1.0.0"}.ToJob()
	require.Equal(t, "refs/tags/v1.0.0", job.HeadBranch)

	job = ScaleSetJobMessage{}.ToJob()
	require.Empty(t, job.WorkflowName)
	require.Empty(t, job.HeadBranch)
}

func TestWorkflowJobPayload(t *testing.T) {
	var job WorkflowJob
	require.NoError(t, json.Unmarshal([]byte(`{"action":"in_progress","workflow_job":{
		"id": 4242, "run_id": 1, "run_attempt": 2,
		"html_url": "https://github.com/jclab/app/actions/runs/1/job/4242",
		"workflow_name": "CI", "head_branch": "main"}}`), &job))
	require.Equal(t, "CI", job.WorkflowJob.WorkflowName)
	require.Equal(t, "main", job.WorkflowJob.HeadBranch)
	require.Equal(t, int64(2), job.WorkflowJob.RunAttempt)
	require.Equal(t, "https://github.com/jclab/app/actions/runs/1/job/4242", job.WorkflowJob.HTMLURL)
}
