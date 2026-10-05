package app

import (
	"context"

	"github.com/uvwt/agentdock/internal/permission"
)

// DesktopPermissionControlCredential is intentionally not exposed through any
// normal tool or Runtime API route. The local native Desktop bootstrap path is
// the only intended caller.
func (r *Runtime) DesktopPermissionControlCredential() string {
	if r == nil || r.permissionCtl == nil {
		return ""
	}
	return r.permissionCtl.Credential()
}

func (r *Runtime) BeginPermissionConfirmation(credential string, request permission.ControlMutationRequest) (permission.ConfirmationChallenge, error) {
	if r == nil || r.permissionCtl == nil {
		return permission.ConfirmationChallenge{}, permission.ErrControlUnauthorized
	}
	return r.permissionCtl.BeginConfirmation(credential, request)
}

func (r *Runtime) ApplyPermissionControlMutation(
	ctx context.Context,
	credential string,
	confirmationID string,
	request permission.ControlMutationRequest,
) (Result, error) {
	if r == nil || r.permissionCtl == nil || r.permissions == nil {
		return nil, permission.ErrControlUnauthorized
	}
	if ctx == nil {
		ctx = context.Background()
	}
	authorized, err := r.permissionCtl.ConsumeConfirmation(credential, confirmationID, request)
	if err != nil {
		return nil, err
	}
	switch authorized.Kind {
	case permission.ControlMutationUpdatePolicy:
		updated, err := r.permissions.ReplacePolicy(authorized.PolicyRevision, *authorized.Policy)
		if err != nil {
			return nil, err
		}
		return Result{"policy": updated}, nil
	case permission.ControlMutationApproveOnce:
		record, err := r.permissions.ApproveOnce(ctx, desktopPermissionMutation(authorized))
		if err != nil {
			return nil, err
		}
		return Result{"approval": record}, nil
	case permission.ControlMutationApproveWorkspace:
		record, policy, err := r.permissions.ApproveWorkspace(ctx, permission.WorkspaceGrantInput{Mutation: desktopPermissionMutation(authorized)})
		if err != nil {
			return nil, err
		}
		return Result{"approval": record, "policy": policy}, nil
	case permission.ControlMutationReject:
		record, err := r.permissions.Reject(ctx, desktopPermissionMutation(authorized))
		if err != nil {
			return nil, err
		}
		return Result{"approval": record}, nil
	default:
		return nil, permission.ErrControlMutation
	}
}

func desktopPermissionMutation(request permission.ControlMutationRequest) permission.Mutation {
	return permission.Mutation{
		ApprovalID:      request.ApprovalID,
		ApprovalVersion: request.ApprovalVersion,
		PolicyRevision:  request.PolicyRevision,
		Actor:           permission.DesktopControlActor,
	}
}
