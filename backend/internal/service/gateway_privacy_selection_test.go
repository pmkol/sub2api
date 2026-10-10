//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func TestGatewayLegacyPrivacySnapshotFiltering(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		for _, required := range []bool{false, true} {
			name := map[bool]string{false: "single", true: "mixed"}[mixed] + "/" + map[bool]string{false: "disabled", true: "required"}[required]
			t.Run(name, func(t *testing.T) {
				id := int64(10)
				group := &Group{ID: id, Name: "privacy-test", Platform: PlatformGemini, Status: StatusActive, Hydrated: true, RequirePrivacySet: required, RequirePrivacySetLoaded: true}
				ctx := context.WithValue(context.Background(), ctxkey.Group, group)
				repo := &mockAccountRepoForPlatform{accounts: []Account{
					{ID: 1, Platform: PlatformAntigravity, Priority: 1, Status: StatusActive, Schedulable: true, Extra: map[string]any{"mixed_scheduling": true}},
					{ID: 2, Platform: PlatformAntigravity, Priority: 2, Status: StatusActive, Schedulable: true, Extra: map[string]any{"mixed_scheduling": true, "privacy_mode": AntigravityPrivacySet}},
				}}
				groups := &authPrivacyGroupRepoStub{}
				svc := &GatewayService{accountRepo: repo, groupRepo: groups, cache: &mockGatewayCacheForPlatform{}, cfg: testConfig()}
				var got *Account
				var err error
				if mixed {
					got, err = svc.selectAccountWithMixedScheduling(ctx, &id, "", "", nil, PlatformGemini)
				} else {
					got, err = svc.selectAccountForModelWithPlatform(ctx, &id, "", "", nil, PlatformAntigravity)
				}
				require.NoError(t, err)
				require.NotNil(t, got)
				want := int64(1)
				if required {
					want = 2
				}
				require.Equal(t, want, got.ID)
				require.Zero(t, repo.setErrorCalls, "group privacy filtering must not disable shared accounts")
				require.Empty(t, groups.ids, "loaded auth snapshot must avoid the privacy query")
			})
		}
	}
}

func TestGatewayLoadAwarePrivacyFiltering(t *testing.T) {
	for _, platform := range []string{PlatformAnthropic, PlatformGemini, PlatformAntigravity} {
		for _, mode := range []string{"normal", "sticky", "routed", "wait", "all_filtered", "disabled"} {
			t.Run(platform+"/"+mode, func(t *testing.T) {
				id := int64(10)
				group := &Group{ID: id, Platform: platform, Status: StatusActive, Hydrated: true, RequirePrivacySet: mode != "disabled", RequirePrivacySetLoaded: true}
				model := ""
				if mode == "routed" {
					if platform != PlatformAnthropic {
						t.Skip("model routing targets Anthropic/OpenAI")
					}
					model = "claude-sonnet-4-5"
					group.ModelRoutingEnabled = true
					group.ModelRouting = map[string][]int64{model: {1, 2}}
				}
				ctx := context.WithValue(context.Background(), ctxkey.Group, group)
				accounts := []Account{
					{ID: 1, Platform: PlatformAntigravity, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5, Extra: map[string]any{"mixed_scheduling": true}},
					{ID: 2, Platform: PlatformAntigravity, Priority: 2, Status: StatusActive, Schedulable: true, Concurrency: 5, Extra: map[string]any{"mixed_scheduling": true, "privacy_mode": AntigravityPrivacySet}},
				}
				if mode == "all_filtered" {
					accounts = accounts[:1]
				}
				repo := &mockAccountRepoForPlatform{accounts: accounts}
				groups := &authPrivacyGroupRepoStub{}
				cache := &mockGatewayCacheForPlatform{sessionBindings: map[string]int64{"sticky": 1}}
				concurrency := &mockConcurrencyCache{}
				if mode == "wait" {
					concurrency.acquireResults = map[int64]bool{1: false, 2: false}
				}
				cfg := testConfig()
				cfg.Gateway.Scheduling.LoadBatchEnabled = true
				svc := &GatewayService{accountRepo: repo, groupRepo: groups, cache: cache, cfg: cfg, concurrencyService: NewConcurrencyService(concurrency)}
				session := ""
				if mode == "sticky" || mode == "routed" {
					session = "sticky"
				}
				got, err := svc.SelectAccountWithLoadAwareness(ctx, &id, session, model, nil, "", 0)
				if mode == "all_filtered" {
					require.ErrorIs(t, err, ErrNoAvailableAccounts)
					require.Nil(t, got)
					require.Zero(t, concurrency.acquireAccountCalls)
				} else {
					require.NoError(t, err)
					require.NotNil(t, got)
					want := int64(2)
					if mode == "disabled" {
						want = 1
					}
					require.Equal(t, want, got.Account.ID)
					if mode == "wait" {
						require.NotNil(t, got.WaitPlan)
						require.Equal(t, int64(2), got.WaitPlan.AccountID)
					}
					if got.ReleaseFunc != nil {
						got.ReleaseFunc()
					}
				}
				require.Empty(t, groups.ids)
				require.Zero(t, repo.setErrorCalls)
				require.Equal(t, StatusActive, repo.accounts[0].Status)
				require.True(t, repo.accounts[0].Schedulable)
			})
		}
	}
}

func TestGatewayPrivacyHydratedAccountRejected(t *testing.T) {
	ctx := context.WithValue(context.Background(), gatewayPrivacyRequiredContextKey{}, true)
	svc := &GatewayService{}
	released := false
	got, err := svc.newSelectionResult(ctx, &Account{ID: 1, Platform: PlatformAntigravity}, true, func() { released = true }, nil)
	require.ErrorIs(t, err, ErrNoAvailableAccounts)
	require.Nil(t, got)
	require.True(t, released)
}
