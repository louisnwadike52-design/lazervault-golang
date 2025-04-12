package grpcApi

import (
	"context"
	"fmt"
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

	// Create user in database
	if err := services.CreateUser(s.db, user); err != nil {
		return s.createErrorResponse(codes.InvalidArgument, err.Error())
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
	fmt.Println("grpc error: ", errorMessage)
	return &pb.CreateUserResponse{
		Success: false,
		Message: errorMessage,
		User:    nil,
	}, status.Errorf(errorCode, errorMessage)
}
