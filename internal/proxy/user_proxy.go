package proxy

import (
	"context"

	authinterceptor "github.com/lazervault/shared/auth-interceptor"
	pb "lazervaultGo/pb"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// UserServiceProxy proxies UserService gRPC requests to the auth-service
// This enables the Flutter app to fetch user profiles and search users via the gateway
type UserServiceProxy struct {
	pb.UnimplementedUserServiceServer
	authClient pb.AuthServiceClient
}

// NewUserServiceProxy creates a new UserServiceProxy
func NewUserServiceProxy(authClient pb.AuthServiceClient) *UserServiceProxy {
	return &UserServiceProxy{
		authClient: authClient,
	}
}

// GetUserProfile returns the current authenticated user's profile
func (p *UserServiceProxy) GetUserProfile(ctx context.Context, req *pb.GetUserProfileRequest) (*pb.GetUserProfileResponse, error) {
	// Get user ID from JWT context
	userID, err := authinterceptor.GetUserID(ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}

	// Call Auth Service GetMe
	authResp, err := p.authClient.GetMe(forwardContext(ctx), &pb.GetMeRequest{
		UserId: userID,
	})
	if err != nil {
		return nil, err
	}

	if authResp.User == nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	// Map auth.User (from GetMeResponse) to common.User (for GetUserProfileResponse)
	return &pb.GetUserProfileResponse{
		Success: true,
		Message: "Profile retrieved successfully",
		User:    authResp.User,
	}, nil
}

// UpdateUserProfile updates the authenticated user's profile
func (p *UserServiceProxy) UpdateUserProfile(ctx context.Context, req *pb.UpdateUserProfileRequest) (*pb.UpdateUserProfileResponse, error) {
	userID, err := authinterceptor.GetUserID(ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}

	// Call Auth Service UpdateProfile
	authResp, err := p.authClient.UpdateProfile(forwardContext(ctx), &pb.UpdateProfileRequest{
		UserId:         userID,
		FirstName:      req.FirstName,
		LastName:       req.LastName,
		Username:       req.Username,
		Phone:          req.PhoneNumber,
		ProfilePicture: req.ProfilePicture,
	})
	if err != nil {
		return nil, err
	}

	return &pb.UpdateUserProfileResponse{
		Success: authResp.Success,
		Message: authResp.Msg,
		User:    authResp.User,
	}, nil
}

// Note: SearchUserByUsername RPC exists in user.proto but requires proto regeneration
// The Flutter app should use /api/v1/auth/search/users endpoint directly for now

// Stub implementations for other UserService methods
// These redirect to appropriate auth endpoints or return unimplemented

func (p *UserServiceProxy) CreateUser(ctx context.Context, req *pb.CreateUserRequest) (*pb.CreateUserResponse, error) {
	return nil, status.Error(codes.Unimplemented, "use /api/v1/auth/signup instead")
}

func (p *UserServiceProxy) UpdatePassword(ctx context.Context, req *pb.UpdatePasswordRequest) (*pb.UpdatePasswordResponse, error) {
	return nil, status.Error(codes.Unimplemented, "use /api/v1/auth/change-password instead")
}

func (p *UserServiceProxy) UpdatePreferences(ctx context.Context, req *pb.UpdatePreferencesRequest) (*pb.UpdatePreferencesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (p *UserServiceProxy) UploadIDDocument(ctx context.Context, req *pb.UploadIDDocumentRequest) (*pb.UploadIDDocumentResponse, error) {
	return nil, status.Error(codes.Unimplemented, "use /api/v1/auth/verify-identity instead")
}

func (p *UserServiceProxy) GetIDDocuments(ctx context.Context, req *pb.GetIDDocumentsRequest) (*pb.GetIDDocumentsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "use /api/v1/auth/identity-status instead")
}

func (p *UserServiceProxy) VerifyIDDocument(ctx context.Context, req *pb.VerifyIDDocumentRequest) (*pb.VerifyIDDocumentResponse, error) {
	return nil, status.Error(codes.Unimplemented, "use /api/v1/auth/verify-identity instead")
}

func (p *UserServiceProxy) RegisterFace(ctx context.Context, req *pb.UserRegisterFaceRequest) (*pb.UserRegisterFaceResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (p *UserServiceProxy) VerifyFace(ctx context.Context, req *pb.UserVerifyFaceRequest) (*pb.UserVerifyFaceResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (p *UserServiceProxy) GetFacialData(ctx context.Context, req *pb.GetFacialDataRequest) (*pb.GetFacialDataResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (p *UserServiceProxy) SetPasscode(ctx context.Context, req *pb.SetPasscodeRequest) (*pb.SetPasscodeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "use /api/v1/auth/register-passcode instead")
}

func (p *UserServiceProxy) VerifyPasscode(ctx context.Context, req *pb.VerifyPasscodeRequest) (*pb.VerifyPasscodeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (p *UserServiceProxy) RemovePasscode(ctx context.Context, req *pb.RemovePasscodeRequest) (*pb.RemovePasscodeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (p *UserServiceProxy) CheckPasscodeExists(ctx context.Context, req *pb.CheckPasscodeExistsRequest) (*pb.CheckPasscodeExistsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (p *UserServiceProxy) UpdateDevicePermissions(ctx context.Context, req *pb.UpdateDevicePermissionsRequest) (*pb.UpdateDevicePermissionsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (p *UserServiceProxy) GetDevicePermissions(ctx context.Context, req *pb.GetDevicePermissionsRequest) (*pb.GetDevicePermissionsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}
