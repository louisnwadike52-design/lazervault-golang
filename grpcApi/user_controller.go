package grpcApi

import (
	"context"
	"fmt"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"lazervaultGo/services"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type UserController struct {
	pb.UnimplementedUserServiceServer
	server *Server
}

func NewUserController(server *Server) *UserController {
	return &UserController{
		server: server,
	}
}

func (c *UserController) CreateUser(ctx context.Context, req *pb.CreateUserRequest) (*pb.CreateUserResponse, error) {
	// Log request received
	log.Info().
		Str("email", req.Email).
		Str("first_name", req.FirstName).
		Str("last_name", req.LastName).
		Str("phone_number", req.PhoneNumber).
		Str("role", req.Role).
		Msg("Received CreateUser request")

	// Create user model from request
	user := &models.User{
		FirstName:   req.FirstName,
		LastName:    req.LastName,
		Email:       req.Email,
		Password:    &req.Password,
		PhoneNumber: req.PhoneNumber,
		Role:        req.Role,
	}

	userService := services.NewUserService(c.server.db, c.server.config, c.server.tokenMaker)
	// Create user in database
	if err := userService.CreateUser(ctx, user); err != nil {
		// Logging handled in createErrorResponse
		return c.createErrorResponse(codes.InvalidArgument, err.Error(), req.Email)
	}

	// Log success
	log.Info().Uint("user_id", user.ID).Str("email", user.Email).Msg("User created successfully")

	// Convert to protobuf response
	return &pb.CreateUserResponse{
		Success: true,
		Message: "User created successfully",
		Data: &pb.Data{
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
		},
	}, nil
}

func (c *UserController) createErrorResponse(errorCode codes.Code, errorMessage string, email string) (*pb.CreateUserResponse, error) {
	// Log the error
	log.Error().
		Str("email_attempted", email).
		Str("error", errorMessage).
		Str("grpc_code", errorCode.String()).
		Msg("CreateUser failed")

	fmt.Println("grpc error: ", errorMessage)
	return &pb.CreateUserResponse{
		Success: false,
		Message: errorMessage,
		Data:    nil,
	}, status.Errorf(errorCode, errorMessage)
}
