package services

import (
	"context"
	"errors"

	// "fmt"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"log"
	"strconv"

	// "strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// ITxFileService defines the interface for transaction file related operations.
type ITxFileService interface {
	GetUserTxFileUrl(ctx context.Context, userIDStr string) (*pb.GetUserTxFileUrlResponse, error)
	// Remove GetTxFilePath as it's redundant now
	// GetTxFilePath(ctx context.Context, userID uint) (*pb.GetTxFilePathResponse, error)
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

// Remove gcsPathToPublicURL helper

// GetUserTxFileUrl retrieves the stored public URL for the user's transaction file.
func (s *TxFileService) GetUserTxFileUrl(ctx context.Context, userIDStr string) (*pb.GetUserTxFileUrlResponse, error) {
	log.Printf("INFO: GetUserTxFileUrl: Fetching URL for User ID: %s", userIDStr)

	userID, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil {
		log.Printf("ERROR: GetUserTxFileUrl: Invalid User ID format: %s", userIDStr)
		return nil, status.Error(codes.InvalidArgument, "invalid user ID format")
	}

	var fileRecord models.UserTransactionFile
	if err := s.db.WithContext(ctx).Where("user_id = ?", uint(userID)).First(&fileRecord).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("WARN: GetUserTxFileUrl: No transaction file record found for User ID: %d", uint(userID))
			return nil, status.Error(codes.NotFound, "transaction file not found for user")
		}
		log.Printf("ERROR: GetUserTxFileUrl: Failed to query UserTransactionFile for User ID %d: %v", uint(userID), err)
		return nil, status.Error(codes.Internal, "failed to retrieve transaction file path")
	}

	// FilePath now directly contains the public URL
	publicURL := fileRecord.FilePath

	log.Printf("INFO: GetUserTxFileUrl: Returning public URL %s for User ID: %d", publicURL, uint(userID))
	return &pb.GetUserTxFileUrlResponse{
		PublicFileUrl: publicURL,
	}, nil
}

// Remove GetTxFilePath function entirely
