package proxy

import (
	"context"
	"log"
	"strconv"
	"time"

	pb "lazervaultGo/pb"
	notificationspb "notifications-service/proto"

	authinterceptor "github.com/lazervault/shared/auth-interceptor"

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

	// Load the user's real notification/display preferences so the app reflects
	// persisted state (push/email/sms/dark mode) instead of client defaults.
	prefs := &pb.UserPreferences{
		UserId:        userID,
		Country:       authResp.User.GetCountryCode(),
		ActiveCountry: authResp.User.GetCountryCode(),
	}
	if p.notificationsClient != nil {
		if np, nerr := p.notificationsClient.GetNotificationPreferences(forwardContext(ctx),
			&notificationspb.GetNotificationPreferencesRequest{UserId: userID}); nerr == nil && np.GetPreferences() != nil {
			prefs.PushNotifications = np.GetPreferences().GetPushEnabled()
			prefs.EmailNotifications = np.GetPreferences().GetEmailEnabled()
			prefs.SmsNotifications = np.GetPreferences().GetSmsEnabled()
			prefs.DarkMode = np.GetPreferences().GetDarkMode()
		}
	}

	return &pb.GetUserProfileResponse{
		Success:     true,
		Message:     "Profile retrieved successfully",
		User:        commonUser,
		Preferences: prefs,
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

// authUserToCommonUser maps auth-service User (auth.proto) to CommonUser (common.proto).
// The auth.proto User message has different field names and types than common.proto CommonUser.
func authUserToCommonUser(au *pb.User) *pb.CommonUser {
	if au == nil {
		return nil
	}

	// CommonUser.id is a uint64 left over from numeric user ids; this platform
	// uses UUIDs, so this parse ALWAYS fails and the field is always 0. It used
	// to log a warning on every profile fetch — several times a minute, per
	// signed-in user — which is noise, not a signal, because there is no input
	// that would ever make it succeed. The real id now travels in `user_id`
	// (field 17); `id` stays 0 for wire compatibility with installed clients.
	var userID uint64
	if parsed, err := strconv.ParseUint(au.Id, 10, 64); err == nil {
		userID = parsed
	}

	return &pb.CommonUser{
		Id:              userID,
		UserId:          au.Id,
		FirstName:       au.FirstName,
		LastName:        au.LastName,
		Email:           au.Email,
		PhoneNumber:     au.Phone,
		Username:        au.Username,
		// `Verified` is the phone-verified flag on the client model — map it from
		// PhoneVerified (was incorrectly mirrored from EmailVerified).
		Verified:        au.PhoneVerified,
		IsEmailVerified: au.EmailVerified,
		Country:         au.CountryCode,
		ProfilePicture:  au.ProfilePicture,
		CreatedAt:       parseISO8601Timestamp(au.CreatedAt),
		UpdatedAt:       parseISO8601Timestamp(au.UpdatedAt),
		Roles:           au.Roles,
		Role:            au.Role,
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

	// Start from the user's CURRENT preferences so we don't clobber the granular
	// per-type flags (transfers/payments/…) — the app only sets the 3 global
	// channel master switches + dark mode.
	base := &notificationspb.NotificationPreferences{}
	if cur, cerr := p.notificationsClient.GetNotificationPreferences(forwardContext(ctx),
		&notificationspb.GetNotificationPreferencesRequest{UserId: userID}); cerr == nil && cur.GetPreferences() != nil {
		base = cur.GetPreferences()
	}
	base.PushEnabled = req.PushNotifications
	base.EmailEnabled = req.EmailNotifications
	base.SmsEnabled = req.SmsNotifications
	base.DarkMode = req.DarkMode

	notifResp, err := p.notificationsClient.UpdateNotificationPreferences(forwardContext(ctx), &notificationspb.UpdateNotificationPreferencesRequest{
		UserId:      userID,
		Preferences: base,
	})
	if err != nil {
		log.Printf("[UserProxy] Failed to update notification preferences: %v", err)
		return nil, err
	}

	// Return the PERSISTED channel/display values (not echoes of the request).
	saved := notifResp.GetPreferences()
	if saved == nil {
		saved = base
	}
	return &pb.UpdatePreferencesResponse{
		Success: true,
		Message: notifResp.Message,
		Preferences: &pb.UserPreferences{
			UserId:             userID,
			PushNotifications:  saved.GetPushEnabled(),
			EmailNotifications: saved.GetEmailEnabled(),
			SmsNotifications:   saved.GetSmsEnabled(),
			DarkMode:           saved.GetDarkMode(),
			Language:           req.Language,
			Currency:           req.Currency,
			Country:            req.ActiveCountry,
			PreferredCountries: req.PreferredCountries,
			ActiveCountry:      req.ActiveCountry,
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
	// Authenticated, read-only check of the current login passcode (no session
	// change). The user identity is carried in the forwarded JWT metadata, which
	// auth-service reads as x-user-id.
	if _, err := authinterceptor.GetUserID(ctx); err != nil {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}

	authResp, err := p.authClient.VerifyPasscode(forwardContext(ctx), &pb.VerifyPasscodeRequest{
		Passcode: req.Passcode,
	})
	if err != nil {
		return nil, err
	}

	return &pb.VerifyPasscodeResponse{
		Success:           authResp.Success,
		Message:           authResp.Message,
		IsValid:           authResp.IsValid,
		AttemptsRemaining: authResp.AttemptsRemaining,
		RetryAfterSeconds: authResp.RetryAfterSeconds,
	}, nil
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

	// Authoritative: GetMe reports has_passcode from login_passcode_hash != "".
	// (signup_status is unreliable — a user can have a passcode while still at
	// an earlier signup stage, e.g. "created".)
	return &pb.CheckPasscodeExistsResponse{
		Success:     true,
		HasPasscode: authResp.HasPasscode,
	}, nil
}

func (p *UserServiceProxy) UpdateDevicePermissions(ctx context.Context, req *pb.UpdateDevicePermissionsRequest) (*pb.UpdateDevicePermissionsResponse, error) {
	if _, err := authinterceptor.GetUserID(ctx); err != nil {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}
	authReq := &pb.AuthUpdateDevicePermissionsRequest{}
	for _, perm := range req.Permissions {
		grantedAt := ""
		if perm.GrantedAt != nil {
			grantedAt = perm.GrantedAt.AsTime().Format(time.RFC3339)
		}
		authReq.Permissions = append(authReq.Permissions, &pb.AuthDevicePermission{
			PermissionType: perm.PermissionType.String(),
			IsGranted:      perm.IsGranted,
			GrantedAt:      grantedAt,
		})
	}
	authResp, err := p.authClient.UpdateDevicePermissions(forwardContext(ctx), authReq)
	if err != nil {
		return nil, err
	}
	return &pb.UpdateDevicePermissionsResponse{
		Success: authResp.Success,
		Message: authResp.Message,
	}, nil
}

func (p *UserServiceProxy) GetDevicePermissions(ctx context.Context, req *pb.GetDevicePermissionsRequest) (*pb.GetDevicePermissionsResponse, error) {
	if _, err := authinterceptor.GetUserID(ctx); err != nil {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}
	authResp, err := p.authClient.GetDevicePermissions(forwardContext(ctx), &pb.AuthGetDevicePermissionsRequest{})
	if err != nil {
		return nil, err
	}
	out := &pb.GetDevicePermissionsResponse{
		Success: authResp.Success,
		Message: authResp.Message,
	}
	for _, e := range authResp.Permissions {
		var ts *timestamppb.Timestamp
		if e.GrantedAt != "" {
			if t, perr := time.Parse(time.RFC3339, e.GrantedAt); perr == nil {
				ts = timestamppb.New(t)
			}
		}
		out.Permissions = append(out.Permissions, &pb.DevicePermission{
			PermissionType: pb.PermissionType(pb.PermissionType_value[e.PermissionType]),
			IsGranted:      e.IsGranted,
			GrantedAt:      ts,
		})
	}
	return out, nil
}
