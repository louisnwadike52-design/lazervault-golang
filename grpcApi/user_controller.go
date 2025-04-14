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
	// Create user model from request
	user := &models.User{
		FirstName:   req.FirstName,
		LastName:    req.LastName,
		Email:       req.Email,
		Password:    req.Password,
		PhoneNumber: req.PhoneNumber,
		Role:        req.Role,
	}

	userService := services.NewUserService(c.server.db, c.server.config, c.server.tokenMaker)
	// Create user in database
	if err := userService.CreateUser(ctx, user); err != nil {
		return c.createErrorResponse(codes.InvalidArgument, err.Error())
	}

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

func (c *UserController) createErrorResponse(errorCode codes.Code, errorMessage string) (*pb.CreateUserResponse, error) {
	fmt.Println("grpc error: ", errorMessage)
	return &pb.CreateUserResponse{
		Success: false,
		Message: errorMessage,
		Data:    nil,
	}, status.Errorf(errorCode, errorMessage)
}
