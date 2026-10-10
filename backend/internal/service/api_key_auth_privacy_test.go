package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

type authPrivacyGroupRepoStub struct {
	GroupRepository
	required bool
	err      error
	ids      []int64
}

func (r *authPrivacyGroupRepoStub) GetByIDLite(_ context.Context, id int64) (*Group, error) {
	r.ids = append(r.ids, id)
	return &Group{ID: id, RequirePrivacySet: r.required}, r.err
}

func TestAuthSnapshotPrivacyRoundtrip(t *testing.T) {
	for _, required := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "required"}[required], func(t *testing.T) {
			id := int64(10)
			key := &APIKey{GroupID: &id, User: &User{ID: 1}, Group: &Group{ID: id, RequirePrivacySet: required, RequirePrivacySetLoaded: true}}
			auth := &APIKeyService{}
			data, err := json.Marshal(auth.snapshotFromAPIKey(context.Background(), key))
			require.NoError(t, err)
			var snapshot APIKeyAuthSnapshot
			require.NoError(t, json.Unmarshal(data, &snapshot))
			restored := auth.snapshotToAPIKey("key", &snapshot)
			require.True(t, restored.Group.RequirePrivacySetLoaded)
			repo := &authPrivacyGroupRepoStub{err: errors.New("must not query")}
			svc := &OpenAIGatewayService{schedulerSnapshot: &SchedulerSnapshotService{groupRepo: repo}}
			ctx := context.WithValue(context.Background(), ctxkey.Group, restored.Group)
			require.Equal(t, required, svc.openAIGroupRequiresPrivacySet(ctx, &id))
			require.Empty(t, repo.ids)
		})
	}
}

func TestAuthSnapshotPrivacyFallback(t *testing.T) {
	for _, tc := range []struct {
		name     string
		group    *Group
		required bool
		err      error
		want     bool
	}{
		{name: "legacy snapshot", group: &Group{ID: 10}, required: true, want: true},
		{name: "fallback group does not inherit false", group: &Group{ID: 20, RequirePrivacySetLoaded: true}, required: true, want: true},
		{name: "fallback group does not inherit true", group: &Group{ID: 20, RequirePrivacySetLoaded: true, RequirePrivacySet: true}, want: false},
		{name: "read failure is conservative", group: &Group{ID: 10}, err: errors.New("database unavailable"), want: true},
		{name: "no auth group", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := int64(10)
			repo := &authPrivacyGroupRepoStub{required: tc.required, err: tc.err}
			svc := &OpenAIGatewayService{schedulerSnapshot: &SchedulerSnapshotService{groupRepo: repo}}
			ctx := context.WithValue(context.Background(), ctxkey.Group, tc.group)
			ctx = svc.withOpenAIGroupPrivacyRequirement(ctx, &id)
			ctx = svc.withOpenAIGroupPrivacyRequirement(ctx, &id)
			require.Equal(t, tc.want, svc.openAIGroupRequiresPrivacySet(ctx, &id))
			require.Equal(t, []int64{10}, repo.ids)
			other := int64(30)
			_ = svc.openAIGroupRequiresPrivacySet(ctx, &other)
			require.Equal(t, []int64{10, 30}, repo.ids)
		})
	}
}

func TestAuthSnapshotPrivacyLegacyJSON(t *testing.T) {
	var snapshot APIKeyAuthSnapshot
	require.NoError(t, json.Unmarshal([]byte(`{"group":{"id":10}}`), &snapshot))
	key := (&APIKeyService{}).snapshotToAPIKey("key", &snapshot)
	require.False(t, key.Group.RequirePrivacySetLoaded)
	repo := &authPrivacyGroupRepoStub{required: true}
	svc := &OpenAIGatewayService{schedulerSnapshot: &SchedulerSnapshotService{groupRepo: repo}}
	ctx := context.WithValue(context.Background(), ctxkey.Group, key.Group)
	id := int64(10)
	require.True(t, svc.openAIGroupRequiresPrivacySet(ctx, &id))
	require.Equal(t, []int64{10}, repo.ids)
}

func TestGatewayPrivacySelectionSnapshot(t *testing.T) {
	for _, tc := range []struct {
		name      string
		group     *Group
		required  bool
		wantCalls int
	}{
		{"loaded enabled", &Group{ID: 10, Name: "cached", RequirePrivacySet: true, RequirePrivacySetLoaded: true}, true, 0},
		{"loaded disabled", &Group{ID: 10, Name: "cached", RequirePrivacySetLoaded: true}, false, 0},
		{"old snapshot", &Group{ID: 10}, true, 1},
		{"different group", &Group{ID: 20, RequirePrivacySetLoaded: true}, true, 1},
		{"missing snapshot", nil, true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := int64(10)
			repo := &authPrivacyGroupRepoStub{required: tc.required}
			svc := &GatewayService{groupRepo: repo}
			ctx := context.WithValue(context.Background(), ctxkey.Group, tc.group)
			got := svc.groupForPrivacySelection(ctx, &id)
			require.NotNil(t, got)
			require.Equal(t, tc.required, got.RequirePrivacySet)
			require.Len(t, repo.ids, tc.wantCalls)
			if tc.wantCalls == 0 {
				require.Same(t, tc.group, got)
				require.Equal(t, "cached", got.Name)
			}
		})
	}
}

func TestGatewayPrivacySelectionPreservesReadFailure(t *testing.T) {
	id := int64(10)
	repo := &privacyReadFailureRepo{}
	svc := &GatewayService{groupRepo: repo}
	require.Nil(t, svc.groupForPrivacySelection(context.Background(), &id))
	require.Nil(t, svc.groupForPrivacySelection(context.Background(), nil))
	require.Nil(t, (&GatewayService{}).groupForPrivacySelection(context.Background(), &id))
}

type privacyReadFailureRepo struct{ GroupRepository }

func (*privacyReadFailureRepo) GetByIDLite(context.Context, int64) (*Group, error) {
	return nil, errors.New("database unavailable")
}
