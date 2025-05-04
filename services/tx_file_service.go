package services

import (
	"context"
	"errors"
	"fmt"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"strconv"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// ITxFileService defines the interface for transaction file related operations.
type ITxFileService interface {
	GetUserTxFileUrl(ctx context.Context, userIDStr string) (*pb.GetUserTxFileUrlResponse, error)
}

// TxFileService implements the ITxFileService interface.
type TxFileService struct {
	db *gorm.DB
}

// NewTxFileService creates a new TxFileService.
func NewTxFileService(db *gorm.DB) ITxFileService {
	return &TxFileService{
		db: db,
	}
}

// GetUserTxFileUrl retrieves the stored Signed URL for the user's transaction file from the database.
func (s *TxFileService) GetUserTxFileUrl(ctx context.Context, userIDStr string) (*pb.GetUserTxFileUrlResponse, error) {
	if userIDStr == "" {
		return nil, status.Errorf(codes.InvalidArgument, "User ID string cannot be empty")
	}

	// Convert string UserID to uint for DB query
	userID, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user ID format: %v", err)
	}

	var fileRecord models.UserTransactionFile

	// Query the database for the file path (which is the stored Signed URL)
	err = s.db.WithContext(ctx).Where("user_id = ?", uint(userID)).First(&fileRecord).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Errorf(codes.NotFound, "transaction file URL record not found for this user")
		}
		// Log internal DB errors
		fmt.Printf("ERROR: Failed to query UserTransactionFile for user %d: %v\n", userID, err)
		return nil, status.Errorf(codes.Internal, "failed to retrieve transaction file URL")
	}

	// Return the found file path (Public URL)
	return &pb.GetUserTxFileUrlResponse{
		PublicFileUrl: fileRecord.FilePath,
	}, nil
}
