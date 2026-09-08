package proxy

import (
	"context"

	"lazervaultGo/pb"
)

// ContactSyncServiceProxy forwards contact-discovery RPCs to auth-service
// (which owns the users table). Only FindLazerVaultUsers is implemented
// upstream; the rest return Unimplemented from the embedded base.
type ContactSyncServiceProxy struct {
	pb.UnimplementedContactSyncServiceServer
	client pb.ContactSyncServiceClient
}

func NewContactSyncServiceProxy(client pb.ContactSyncServiceClient) *ContactSyncServiceProxy {
	return &ContactSyncServiceProxy{client: client}
}

func (p *ContactSyncServiceProxy) FindLazerVaultUsers(ctx context.Context, req *pb.FindLazerVaultUsersRequest) (*pb.FindLazerVaultUsersResponse, error) {
	return p.client.FindLazerVaultUsers(forwardContext(ctx), req)
}
