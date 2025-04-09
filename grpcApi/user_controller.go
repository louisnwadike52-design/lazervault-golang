package grpcApi

import (
	"context"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"lazervaultGo/services"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *GRPCServer) CreateUser(ctx context.Context, req *pb.CreateUserRequest) (*pb.CreateUserResponse, error) {
	// Create user model from request
	user := &models.User{
		FirstName:   req.FirstName,
		LastName:    req.LastName,
		Email:       req.Email,
		Password:    req.Password,
		PhoneNumber: req.PhoneNumber,
		Role:        req.Role,
	}

	// // Validate input
	// if validationErrors := validators.ValidateStruct(user); validationErrors != nil {
	// 	return nil, status.Errorf(codes.InvalidArgument, "Validation failed: %v", validationErrors)
	// }

	// validate user
	// if err := validators.ValidateUser(user); err != nil {
	// 	return nil, status.Errorf(codes.InvalidArgument, "Validation failed: %v", err)
	// }

	// Create user in database
	// if err := s.db.Create(user).Error; err != nil {
	// 	switch err {
	// 	case models.ErrDuplicateEmail:
	// 		return nil, status.Errorf(codes.AlreadyExists, "Email already exists")
	// 	case models.ErrDuplicatePhone:
	// 		return nil, status.Errorf(codes.AlreadyExists, "Phone number already exists")
	// 	default:
	// 		return nil, status.Errorf(codes.Internal, "Failed to create user: %v", err)
	// 	}
	// }

	// Create user in database
	if err := services.CreateUser(s.db, user); err != nil {
		switch err {
		case models.ErrDuplicateEmail:
			return s.createErrorResponse(codes.AlreadyExists, "Email already exists")
		case models.ErrDuplicatePhone:
			return s.createErrorResponse(codes.AlreadyExists, "Phone number already exists")
		default:
			return s.createErrorResponse(codes.Internal, "Failed to create user")
		}
	}

	// Convert to protobuf response
	return &pb.CreateUserResponse{
		Success: true,
		Message: "User created successfully",
		User: &pb.User{
			Id:          uint64(user.ID),
			FirstName:   user.FirstName,
			LastName:    user.LastName,
			Email:       user.Email,
			PhoneNumber: user.PhoneNumber,
			Role:        user.Role,
			Verified:    user.Verified,
			CreatedAt:   timestamppb.New(user.CreatedAt),
			UpdatedAt:   timestamppb.New(user.UpdatedAt),
		},
	}, nil
}

func (s *GRPCServer) createErrorResponse(errorCode codes.Code, errorMessage string) (*pb.CreateUserResponse, error) {
	return &pb.CreateUserResponse{
		Success: false,
		Message: errorMessage,
		User:    nil,
	}, status.Errorf(errorCode, errorMessage)
}
