package proxy

import (
	"context"
	"log"
	"strconv"
	"time"

	authinterceptor "github.com/lazervault/shared/auth-interceptor"
	pb "lazervaultGo/pb"
	notificationspb "notifications-service/proto"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// UserServiceProxy proxies UserService gRPC requests to the auth-service
// This enables the Flutter app to fetch user profiles and search users via the gateway
type UserServiceProxy struct {
	pb.UnimplementedUserServiceServer
	authClient          pb.AuthServiceClient
	notificationsClient notificationspb.NotificationsServiceClient
}

// NewUserServiceProxy creates a new UserServiceProxy
func NewUserServiceProxy(authClient pb.AuthServiceClient, notificationsClient notificationspb.NotificationsServiceClient) *UserServiceProxy {
	return &UserServiceProxy{
		authClient:          authClient,
		notificationsClient: notificationsClient,
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

	// Map AuthUser (auth-service format) to common.User (client format)
	commonUser := authUserToCommonUser(authResp.User)

	return &pb.GetUserProfileResponse{
		Success: true,
		Message: "Profile retrieved successfully",
		User:    commonUser,
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

	// Map AuthUser (auth-service format) to common.User (client format)
	commonUser := authUserToCommonUser(authResp.User)

	return &pb.UpdateUserProfileResponse{
		Success: authResp.Success,
		Message: authResp.Msg,
		User:    commonUser,
	}, nil
}

// authUserToCommonUser maps auth-service AuthUser fields to common.proto User fields.
// The auth-service User message has a different field layout than common.proto User.
func authUserToCommonUser(au *pb.AuthUser) *pb.User {
	if au == nil {
		return nil
	}

	// Parse string ID to uint64
	var userID uint64
	if au.Id != "" {
		parsed, err := strconv.ParseUint(au.Id, 10, 64)
		if err != nil {
			log.Printf("[UserProxy] Warning: failed to parse user ID %q: %v", au.Id, err)
		} else {
			userID = parsed
		}
	}

	return &pb.User{
		Id:              userID,
		FirstName:       au.FirstName,
		LastName:        au.LastName,
		Email:           au.Email,
		PhoneNumber:     au.Phone,
		Username:        au.Username,
		Verified:        au.EmailVerified,
		IsEmailVerified: au.EmailVerified,
		Country:         au.CountryCode,
		ProfilePicture:  au.ProfilePicture,
		CreatedAt:       parseISO8601Timestamp(au.CreatedAt),
		UpdatedAt:       parseISO8601Timestamp(au.UpdatedAt),
	}
}

// parseISO8601Timestamp converts an ISO 8601 string to a protobuf Timestamp.
func parseISO8601Timestamp(s string) *timestamppb.Timestamp {
	if s == "" {
		return nil
	}
	// Try common ISO 8601 formats
	for _, layout := range []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
	} {
		t, err := time.Parse(layout, s)
		if err == nil {
			return timestamppb.New(t)
		}
	}
	log.Printf("[UserProxy] Warning: failed to parse timestamp %q", s)
	return nil
}

// Note: SearchUserByUsername is handled via AuthService gRPC directly from Flutter

// Stub implementations for other UserService methods
// These redirect to appropriate auth endpoints or return unimplemented

func (p *UserServiceProxy) CreateUser(ctx context.Context, req *pb.CreateUserRequest) (*pb.CreateUserResponse, error) {
	return nil, status.Error(codes.Unimplemented, "use /api/v1/auth/signup instead")
}

func (p *UserServiceProxy) UpdatePassword(ctx context.Context, req *pb.UpdatePasswordRequest) (*pb.UpdatePasswordResponse, error) {
	return nil, status.Error(codes.Unimplemented, "use /api/v1/auth/change-password instead")
}

func (p *UserServiceProxy) UpdatePreferences(ctx context.Context, req *pb.UpdatePreferencesRequest) (*pb.UpdatePreferencesResponse, error) {
	userID, err := authinterceptor.GetUserID(ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}

	if p.notificationsClient == nil {
		return nil, status.Error(codes.Unavailable, "notifications service not available")
	}

	// Proxy notification preferences to notifications-service
	notifResp, err := p.notificationsClient.UpdateNotificationPreferences(forwardContext(ctx), &notificationspb.UpdateNotificationPreferencesRequest{
		UserId: userID,
		Preferences: &notificationspb.NotificationPreferences{
			TransfersEnabled:      req.PushNotifications,
			PaymentsEnabled:       req.PushNotifications,
			DepositsEnabled:       req.PushNotifications,
			WithdrawalsEnabled:    req.PushNotifications,
			AccountUpdatesEnabled: req.EmailNotifications,
			SecurityAlertsEnabled: req.SmsNotifications,
		},
	})
	if err != nil {
		log.Printf("[UserProxy] Failed to update notification preferences: %v", err)
		return nil, err
	}

	return &pb.UpdatePreferencesResponse{
		Success: true,
		Message: notifResp.Message,
		Preferences: &pb.UserPreferences{
			UserId:            userID,
			PushNotifications: req.PushNotifications,
			EmailNotifications: req.EmailNotifications,
			SmsNotifications:  req.SmsNotifications,
			DarkMode:          req.DarkMode,
			Language:          req.Language,
			Currency:          req.Currency,
			Country:           req.ActiveCountry,
			PreferredCountries: req.PreferredCountries,
			ActiveCountry:     req.ActiveCountry,
		},
	}, nil
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
	userID, err := authinterceptor.GetUserID(ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}

	authResp, err := p.authClient.GetMe(forwardContext(ctx), &pb.GetMeRequest{
		UserId: userID,
	})
	if err != nil {
		return nil, err
	}

	if authResp.User == nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	// Passcode is set if signup_status indicates passcode_set or complete
	hasPasscode := authResp.User.SignupStatus == "passcode_set" ||
		authResp.User.SignupStatus == "complete"

	return &pb.CheckPasscodeExistsResponse{
		Success:     true,
		HasPasscode: hasPasscode,
	}, nil
}

func (p *UserServiceProxy) UpdateDevicePermissions(ctx context.Context, req *pb.UpdateDevicePermissionsRequest) (*pb.UpdateDevicePermissionsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (p *UserServiceProxy) GetDevicePermissions(ctx context.Context, req *pb.GetDevicePermissionsRequest) (*pb.GetDevicePermissionsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}
