package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"lazervaultGo/configs"
	"lazervaultGo/models"

	qrcode "github.com/skip2/go-qrcode"
	"gorm.io/gorm"
)

// QRCodeData represents the data structure for LazerVault recipient QR codes
type QRCodeData struct {
	Type        string `json:"type"`
	RecipientID string `json:"recipientId"`
	Username    string `json:"username"`
	Name        string `json:"name"`
	Version     string `json:"version"`
}

// QRCodeResponse represents the response for QR code generation
type QRCodeResponse struct {
	Type        string `json:"type"`
	RecipientID string `json:"recipientId"`
	Username    string `json:"username"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	QRCodeImage string `json:"qrCodeImage"` // Base64 encoded PNG
}

// IQRCodeService defines the interface for QR code operations
type IQRCodeService interface {
	GenerateUserQRCode(ctx context.Context, userID uint) (*QRCodeResponse, error)
	GenerateUserQRCodeBytes(ctx context.Context, userID uint) ([]byte, error)
}

type QRCodeService struct {
	db     *gorm.DB
	config *configs.Config
}

// Ensure QRCodeService implements IQRCodeService
var _ IQRCodeService = (*QRCodeService)(nil)

func NewQRCodeService(db *gorm.DB, config *configs.Config) IQRCodeService {
	return &QRCodeService{
		db:     db,
		config: config,
	}
}

// GenerateUserQRCode generates a QR code for a user with base64 encoded image
func (s *QRCodeService) GenerateUserQRCode(ctx context.Context, userID uint) (*QRCodeResponse, error) {
	// Fetch user from database
	var user models.User
	if err := s.db.WithContext(ctx).First(&user, userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to fetch user: %w", err)
	}

	// Create QR code data (use email as username since User doesn't have username field)
	qrData := QRCodeData{
		Type:        "lazervault_recipient",
		RecipientID: fmt.Sprintf("%d", user.ID),
		Username:    user.Email,
		Name:        s.getUserDisplayName(&user),
		Version:     "1.0",
	}

	// Convert to JSON
	jsonData, err := json.Marshal(qrData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal QR data: %w", err)
	}

	// Generate QR code image (256x256 pixels, medium error correction)
	qrBytes, err := qrcode.Encode(string(jsonData), qrcode.Medium, 256)
	if err != nil {
		return nil, fmt.Errorf("failed to generate QR code: %w", err)
	}

	// Encode to base64
	base64Image := base64.StdEncoding.EncodeToString(qrBytes)

	return &QRCodeResponse{
		Type:        qrData.Type,
		RecipientID: qrData.RecipientID,
		Username:    qrData.Username,
		Name:        qrData.Name,
		Version:     qrData.Version,
		QRCodeImage: base64Image,
	}, nil
}

// GenerateUserQRCodeBytes generates a QR code for a user and returns raw PNG bytes
func (s *QRCodeService) GenerateUserQRCodeBytes(ctx context.Context, userID uint) ([]byte, error) {
	// Fetch user from database
	var user models.User
	if err := s.db.WithContext(ctx).First(&user, userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to fetch user: %w", err)
	}

	// Create QR code data (use email as username since User doesn't have username field)
	qrData := QRCodeData{
		Type:        "lazervault_recipient",
		RecipientID: fmt.Sprintf("%d", user.ID),
		Username:    user.Email,
		Name:        s.getUserDisplayName(&user),
		Version:     "1.0",
	}

	// Convert to JSON
	jsonData, err := json.Marshal(qrData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal QR data: %w", err)
	}

	// Generate QR code image (256x256 pixels, medium error correction)
	qrBytes, err := qrcode.Encode(string(jsonData), qrcode.Medium, 256)
	if err != nil {
		return nil, fmt.Errorf("failed to generate QR code: %w", err)
	}

	return qrBytes, nil
}

// getUserDisplayName gets the best display name for a user
func (s *QRCodeService) getUserDisplayName(user *models.User) string {
	// Priority: FirstName LastName > FirstName > Email
	if user.FirstName != "" && user.LastName != "" {
		return user.FirstName + " " + user.LastName
	}

	if user.FirstName != "" {
		return user.FirstName
	}

	return user.Email
}
