package manager

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/absmach/propeller/pkg/mqtt/mocks"
	"github.com/absmach/propeller/pkg/proplet"
	"github.com/absmach/propeller/pkg/scheduler"
	"github.com/absmach/propeller/pkg/storage"
	"github.com/absmach/propeller/pkg/task"
	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	testTenantID  = "test-tenant"
	testChannelID = "test-channel"
	testBaseTopic = "m/test-tenant/c/test-channel"
	testInvokeRes = testBaseTopic + "/control/proplet/invoke_results"
)

func TestInvokeBroadcastRetries(t *testing.T) {
	t.Parallel()

	const notPrecompiled = "task x has not been precompiled; deploy it as a latent task first"

	t.Run("skips stale proplet and succeeds on healthy one", func(t *testing.T) {
		t.Parallel()

		staleID := uuid.NewString()
		healthyID := uuid.NewString()

		svc, _, _ := broadcastInvokeService(t, func(propletID string) (string, string) {
			if propletID == staleID {
				return "", notPrecompiled
			}
			return "hello from " + healthyID, ""
		}, staleID, healthyID)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		created, err := svc.CreateTask(ctx, task.Task{
			Name:      "latent-fn",
			Latent:    true,
			Broadcast: true,
		})
		require.NoError(t, err)

		results, err := svc.InvokeTask(ctx, created.ID, []string{"world"}, nil)
		require.NoError(t, err)
		require.Equal(t, "hello from "+healthyID, results)
	})

	t.Run("re-deploys latent when every proplet lost the precompiled cache", func(t *testing.T) {
		t.Parallel()

		staleA := uuid.NewString()
		staleB := uuid.NewString()

		svc, _, redeploys := broadcastInvokeService(t, func(string) (string, string) {
			return "", notPrecompiled
		}, staleA, staleB)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		created, err := svc.CreateTask(ctx, task.Task{
			Name:      "latent-fn",
			Latent:    true,
			Broadcast: true,
		})
		require.NoError(t, err)

		_, err = svc.InvokeTask(ctx, created.ID, []string{"world"}, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "has not been precompiled")

		got := redeploys()
		require.Len(t, got, 2)
		ids := map[string]bool{}
		for _, r := range got {
			require.False(t, r["broadcast"].(bool), "re-deploy must be targeted, not broadcast")
			pid := r["proplet_id"].(string)
			require.NotEmpty(t, pid)
			ids[pid] = true
		}
		require.True(t, ids[staleA])
		require.True(t, ids[staleB])
	})
}

func broadcastInvokeService(
	t *testing.T,
	respond func(propletID string) (results, errMsg string),
	propletIDs ...string,
) (*service, *mocks.MockPubSub, func() []map[string]any) {
	t.Helper()

	repos, err := storage.NewRepositories(storage.Config{Type: "memory"})
	require.NoError(t, err)
	for _, id := range propletIDs {
		require.NoError(t, repos.Proplets.Create(context.Background(), proplet.Proplet{
			ID:           id,
			Name:         id,
			AliveHistory: []time.Time{time.Now()},
		}))
	}

	pubsub := mocks.NewMockPubSub(t)

	svc, _, _ := NewService(
		repos, scheduler.NewRoundRobin(), pubsub,
		testTenantID, testChannelID, "", slog.Default(), nil,
	)
	s := svc.(*service)

	var mu sync.Mutex
	var redeployPayloads []map[string]any

	pubsub.On("Publish", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			topic, _ := args.Get(1).(string)
			switch {
			case strings.HasSuffix(topic, "/control/manager/invoke"):
				payload, _ := args.Get(2).(map[string]any)
				invocationID, _ := payload["invocation_id"].(string)
				propletID, _ := payload["proplet_id"].(string)
				if invocationID == "" {
					return
				}
				results, errMsg := respond(propletID)
				msg := map[string]any{"invocation_id": invocationID}
				if errMsg != "" {
					msg["error"] = errMsg
				}
				if results != "" {
					msg["results"] = results
				}
				_ = s.handle(context.Background())(testInvokeRes, msg)
			case strings.HasSuffix(topic, "/control/manager/start"):
				payload, _ := args.Get(2).(startPayload)
				if !payload.Latent || payload.Broadcast {
					return
				}
				mu.Lock()
				redeployPayloads = append(redeployPayloads, map[string]any{
					"broadcast":  payload.Broadcast,
					"proplet_id": payload.PropletID,
				})
				mu.Unlock()
			}
		}).
		Return(nil).Maybe()
	pubsub.On("Subscribe", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	pubsub.On("Unsubscribe", mock.Anything, mock.Anything).Return(nil).Maybe()
	pubsub.On("Disconnect", mock.Anything).Return(nil).Maybe()

	return s, pubsub, func() []map[string]any {
		mu.Lock()
		defer mu.Unlock()
		out := make([]map[string]any, len(redeployPayloads))
		copy(out, redeployPayloads)
		return out
	}
}
