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
