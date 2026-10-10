package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

// authGroupWithPrivacyFromContext only accepts a loaded privacy requirement for
// the requested group. Old snapshots and fallback groups must query the database.
func authGroupWithPrivacyFromContext(ctx context.Context, groupID *int64) *Group {
	if groupID == nil {
		return nil
	}
	group, _ := ctx.Value(ctxkey.Group).(*Group)
	if group != nil && group.ID == *groupID && group.RequirePrivacySetLoaded {
		return group
	}
	return nil
}

func (s *GatewayService) groupForPrivacySelection(ctx context.Context, groupID *int64) *Group {
	if group := authGroupWithPrivacyFromContext(ctx, groupID); group != nil {
		return group
	}
	if groupID == nil || s.groupRepo == nil {
		return nil
	}
	// Preserve the legacy selection path's handling of database read errors.
	group, _ := s.groupRepo.GetByIDLite(ctx, *groupID)
	return group
}

// gatewayPrivacyRequiredContextKey carries the requirement through account hydration.
type gatewayPrivacyRequiredContextKey struct{}
