package api

import (
	"testing"

	"github.com/absmach/propeller/pkg/proplet"
	"github.com/absmach/propeller/pkg/task"
	"github.com/stretchr/testify/assert"
)

func TestListEntityReqValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		desc string
		req  listEntityReq
		err  error
	}{
		{
			desc: "empty status is allowed",
			req: listEntityReq{
				statusFilter: propletStatusFilter,
			},
			err: nil,
		},
		{
			desc: "valid proplet status",
			req: listEntityReq{
				status:       proplet.ActiveStatus.String(),
				statusFilter: propletStatusFilter,
			},
			err: nil,
		},
		{
			desc: "valid job status",
			req: listEntityReq{
				status:       task.RunningStatus.String(),
				statusFilter: jobStatusFilter,
			},
			err: nil,
		},
		{
			desc: "tasks do not support status filtering",
			req: listEntityReq{
				status: task.PendingStatus.String(),
			},
			err: errStatusFilterUnsupported,
		},
		{
			desc: "invalid proplet status",
			req: listEntityReq{
				status:       "mystery",
				statusFilter: propletStatusFilter,
			},
			err: proplet.ErrInvalidStatus,
		},
		{
			desc: "invalid job status",
			req: listEntityReq{
				status:       "mystery",
				statusFilter: jobStatusFilter,
			},
			err: task.ErrInvalidJobStatus,
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()

			err := tc.req.validate()
			assert.Equal(t, tc.err, err, "%s: expected %v got %v", tc.desc, tc.err, err)
		})
	}
}

func TestTaskReqValidateElasticConfig(t *testing.T) {
	t.Parallel()

	cases := []struct {
		desc     string
		metadata task.Metadata
		wantErr  bool
	}{
		{
			desc:     "no metadata",
			metadata: nil,
			wantErr:  false,
		},
		{
			desc:     "plain labels are untouched",
			metadata: task.Metadata{"team": "elastic", "tier": 2},
			wantErr:  false,
		},
		{
			desc: "valid runtime config",
			metadata: task.Metadata{task.MetadataElasticKey: map[string]any{
				task.ElasticWasiSecurity: "arguments = [\"--verbose\"]",
				task.ElasticWasiPEP:      "pep-1",
			}},
			wantErr: false,
		},
		{
			desc: "unknown keys in the reserved map are allowed",
			metadata: task.Metadata{task.MetadataElasticKey: map[string]any{
				"future_key": 42,
			}},
			wantErr: false,
		},
		{
			desc:     "reserved key must be an object",
			metadata: task.Metadata{task.MetadataElasticKey: "wasi_security=policy.toml"},
			wantErr:  true,
		},
		{
			desc: "policy must be a string",
			metadata: task.Metadata{task.MetadataElasticKey: map[string]any{
				task.ElasticWasiSecurity: map[string]any{"arguments": []string{"--verbose"}},
			}},
			wantErr: true,
		},
		{
			desc: "pep must be a string",
			metadata: task.Metadata{task.MetadataElasticKey: map[string]any{
				task.ElasticWasiPEP: 7,
			}},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()

			req := taskReq{task.Task{Name: "task", Metadata: tc.metadata}}
			err := req.validate()
			if tc.wantErr {
				assert.Error(t, err, tc.desc)

				return
			}
			assert.NoError(t, err, tc.desc)
		})
	}
}

func TestTaskElasticConfigRoundTrip(t *testing.T) {
	t.Parallel()

	policy := "arguments = [\"--verbose\"]"
	tsk := task.Task{Metadata: task.Metadata{
		"team": "elastic",
		task.MetadataElasticKey: map[string]any{
			task.ElasticWasiSecurity: policy,
		},
	}}

	cfg := tsk.ElasticConfig()
	assert.Equal(t, policy, cfg[task.ElasticWasiSecurity])
	assert.NotContains(t, cfg, "team", "plain labels must not reach the proplet")

	assert.Nil(t, task.Task{}.ElasticConfig())
	assert.Nil(t, task.Task{Metadata: task.Metadata{"team": "elastic"}}.ElasticConfig())
}
