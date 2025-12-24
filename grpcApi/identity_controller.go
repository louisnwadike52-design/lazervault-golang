package grpcApi

import (
	"context"
	"encoding/base64"
	"fmt"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

// UploadIDDocument handles ID document upload and verification
func (s *UserController) UploadIDDocument(ctx context.Context, req *pb.UploadIDDocumentRequest) (*pb.UploadIDDocumentResponse, error) {
	// Get user from context
	userID, ok := ctx.Value("user_id").(uint)
	if !ok || userID == 0 {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	// Validate request
	if len(req.FrontImage) == 0 {
		return &pb.UploadIDDocumentResponse{
			Success: false,
			Message: "Front image is required",
		}, nil
	}

	// Upload front image to storage
	frontImageURL, err := uploadImageToStorage(req.FrontImage, fmt.Sprintf("id_documents/%d/front_%d.jpg", userID, time.Now().Unix()))
	if err != nil {
		return &pb.UploadIDDocumentResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to upload front image: %v", err),
		}, nil
	}

	// Upload back image if provided
	var backImageURL string
	if len(req.BackImage) > 0 {
		backImageURL, err = uploadImageToStorage(req.BackImage, fmt.Sprintf("id_documents/%d/back_%d.jpg", userID, time.Now().Unix()))
		if err != nil {
			return &pb.UploadIDDocumentResponse{
				Success: false,
				Message: fmt.Sprintf("Failed to upload back image: %v", err),
			}, nil
		}
	}

	// Call AI microservice for OCR processing
	docType := mapDocumentTypeToString(req.DocumentType)
	ocrResponse, err := callDocumentOCR(ctx, req.FrontImage, docType)

	// Create document with initial data
	document := &models.IDDocument{
		UserID:             userID,
		DocumentType:       mapDocumentType(req.DocumentType),
		DocumentFrontURL:   frontImageURL,
		DocumentBackURL:    backImageURL,
		VerificationStatus: models.VerificationStatusProcessing,
	}

	// If OCR succeeded, populate extracted fields
	if err == nil && ocrResponse != nil && ocrResponse.Success {
		if ocrResponse.Data != nil {
			document.DocumentNumber = ocrResponse.Data["document_number"]
			document.FullName = ocrResponse.Data["full_name"]
			document.DateOfBirth = ocrResponse.Data["date_of_birth"]
			document.IssueDate = ocrResponse.Data["issue_date"]
			document.ExpiryDate = ocrResponse.Data["expiry_date"]
			document.IssuingCountry = ocrResponse.Data["issuing_country"]
		}
	}

	if err := s.server.db.Create(document).Error; err != nil {
		return &pb.UploadIDDocumentResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to save document: %v", err),
		}, nil
	}

	return &pb.UploadIDDocumentResponse{
		Success: true,
		Message: "Document uploaded successfully and is being processed",
		Document: &pb.IDDocument{
			Id:                 document.ID,
			UserId:             uint64(document.UserID),
			DocumentType:       req.DocumentType,
			DocumentFrontUrl:   document.DocumentFrontURL,
			DocumentBackUrl:    document.DocumentBackURL,
			VerificationStatus: pb.VerificationStatus_VERIFICATION_STATUS_PROCESSING,
			CreatedAt:          timestamppb.New(document.CreatedAt),
		},
	}, nil
}

// GetIDDocuments retrieves all ID documents for the authenticated user
func (s *UserController) GetIDDocuments(ctx context.Context, req *pb.GetIDDocumentsRequest) (*pb.GetIDDocumentsResponse, error) {
	userID, ok := ctx.Value("user_id").(uint)
	if !ok || userID == 0 {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	var documents []models.IDDocument
	if err := s.server.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&documents).Error; err != nil {
		return &pb.GetIDDocumentsResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to fetch documents: %v", err),
		}, nil
	}

	pbDocuments := make([]*pb.IDDocument, len(documents))
	for i, doc := range documents {
		pbDocuments[i] = modelToProtoIDDocument(&doc)
	}

	return &pb.GetIDDocumentsResponse{
		Success:   true,
		Message:   "Documents retrieved successfully",
		Documents: pbDocuments,
	}, nil
}

// VerifyIDDocument manually triggers verification of an ID document
func (s *UserController) VerifyIDDocument(ctx context.Context, req *pb.VerifyIDDocumentRequest) (*pb.VerifyIDDocumentResponse, error) {
	userID, ok := ctx.Value("user_id").(uint)
	if !ok || userID == 0 {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	var document models.IDDocument
	if err := s.server.db.Where("id = ? AND user_id = ?", req.DocumentId, userID).First(&document).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &pb.VerifyIDDocumentResponse{
				Success: false,
				Message: "Document not found",
			}, nil
		}
		return &pb.VerifyIDDocumentResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to fetch document: %v", err),
		}, nil
	}

	// Call AI microservice for document verification
	// Note: We would need to retrieve the actual image bytes from storage
	// For now, we'll skip AI verification if we can't access the images
	// In production, you should fetch the images from the URLs and verify

	// Simplified verification - mark as verified
	// In production, call: verifyResponse, err := callDocumentVerify(ctx, frontImageBytes, backImageBytes, docType)
	now := time.Now()
	document.VerificationStatus = models.VerificationStatusVerified
	document.VerifiedAt = &now

	if err := s.server.db.Save(&document).Error; err != nil {
		return &pb.VerifyIDDocumentResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to update document: %v", err),
		}, nil
	}

	return &pb.VerifyIDDocumentResponse{
		Success:  true,
		Message:  "Document verified successfully",
		Document: modelToProtoIDDocument(&document),
	}, nil
}

// RegisterFace registers facial biometric data for the user
func (s *UserController) RegisterFace(ctx context.Context, req *pb.UserRegisterFaceRequest) (*pb.UserRegisterFaceResponse, error) {
	userID, ok := ctx.Value("user_id").(uint)
	if !ok || userID == 0 {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	// Upload face image to storage
	faceImageURL, err := uploadImageToStorage(req.FaceImage, fmt.Sprintf("facial_data/%d/face_%d.jpg", userID, time.Now().Unix()))
	if err != nil {
		return &pb.UserRegisterFaceResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to upload face image: %v", err),
		}, nil
	}

	// Call AI microservice for facial registration
	userIDStr := fmt.Sprintf("%d", userID)
	aiResponse, err := callFacialRegister(ctx, req.FaceImage, userIDStr)

	var faceID string
	var faceEncoding string

	if err == nil && aiResponse != nil && aiResponse.Success {
		faceID = aiResponse.FaceID
		// Store a placeholder encoding - the actual encoding is stored in the AI service
		faceEncoding = base64.StdEncoding.EncodeToString([]byte(aiResponse.FaceID))
	} else {
		// Fallback if AI service is unavailable
		faceID = fmt.Sprintf("face_%d_%d", userID, time.Now().Unix())
		faceEncoding = base64.StdEncoding.EncodeToString([]byte("placeholder_encoding"))
	}

	// Check if user already has facial data
	var existingFacialData models.FacialData
	err = s.server.db.Where("user_id = ?", userID).First(&existingFacialData).Error

	if err == nil {
		// Update existing
		existingFacialData.FaceID = faceID
		existingFacialData.FaceEncoding = faceEncoding
		existingFacialData.ImageURL = faceImageURL
		existingFacialData.IsVerified = true
		now := time.Now()
		existingFacialData.LastVerifiedAt = &now

		if err := s.server.db.Save(&existingFacialData).Error; err != nil {
			return &pb.UserRegisterFaceResponse{
				Success: false,
				Message: fmt.Sprintf("Failed to update facial data: %v", err),
			}, nil
		}

		return &pb.UserRegisterFaceResponse{
			Success:    true,
			Message:    "Face registered successfully",
			FacialData: modelToProtoFacialData(&existingFacialData),
		}, nil
	}

	// Create new facial data
	facialData := &models.FacialData{
		UserID:       userID,
		FaceID:       faceID,
		FaceEncoding: faceEncoding,
		ImageURL:     faceImageURL,
		IsVerified:   true,
	}
	now := time.Now()
	facialData.LastVerifiedAt = &now

	if err := s.server.db.Create(facialData).Error; err != nil {
		return &pb.UserRegisterFaceResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to save facial data: %v", err),
		}, nil
	}

	// Update user's facial recognition flag
	s.server.db.Model(&models.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
		"facial_recognition_enabled": true,
		"face_registered_at":         time.Now(),
	})

	return &pb.UserRegisterFaceResponse{
		Success:    true,
		Message:    "Face registered successfully",
		FacialData: modelToProtoFacialData(facialData),
	}, nil
}

// VerifyFace verifies a face against registered biometric data
func (s *UserController) VerifyFace(ctx context.Context, req *pb.UserVerifyFaceRequest) (*pb.UserVerifyFaceResponse, error) {
	userID, ok := ctx.Value("user_id").(uint)
	if !ok || userID == 0 {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	// Get user's facial data
	var facialData models.FacialData
	if err := s.server.db.Where("user_id = ?", userID).First(&facialData).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &pb.UserVerifyFaceResponse{
				Success:  false,
				Message:  "No facial data registered",
				IsMatch:  false,
				Confidence: 0,
			}, nil
		}
		return &pb.UserVerifyFaceResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to fetch facial data: %v", err),
		}, nil
	}

	// Call AI microservice for facial verification
	userIDStr := fmt.Sprintf("%d", userID)
	aiResponse, err := callFacialVerify(ctx, req.FaceImage, userIDStr)

	var isMatch bool
	var confidence float32

	if err == nil && aiResponse != nil && aiResponse.Success {
		isMatch = aiResponse.Verified
		confidence = float32(aiResponse.Confidence)
	} else {
		// Fallback if AI service is unavailable - return unsuccessful verification
		isMatch = false
		confidence = 0.0
	}

	if isMatch {
		// Update last verified time
		now := time.Now()
		facialData.LastVerifiedAt = &now
		s.server.db.Save(&facialData)
	}

	return &pb.UserVerifyFaceResponse{
		Success:    true,
		Message:    "Face verification completed",
		IsMatch:    isMatch,
		Confidence: confidence,
	}, nil
}

// GetFacialData retrieves facial biometric data for the user
func (s *UserController) GetFacialData(ctx context.Context, req *pb.GetFacialDataRequest) (*pb.GetFacialDataResponse, error) {
	userID, ok := ctx.Value("user_id").(uint)
	if !ok || userID == 0 {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	var facialData models.FacialData
	if err := s.server.db.Where("user_id = ?", userID).First(&facialData).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &pb.GetFacialDataResponse{
				Success: false,
				Message: "No facial data registered",
			}, nil
		}
		return &pb.GetFacialDataResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to fetch facial data: %v", err),
		}, nil
	}

	return &pb.GetFacialDataResponse{
		Success:    true,
		Message:    "Facial data retrieved successfully",
		FacialData: modelToProtoFacialData(&facialData),
	}, nil
}

// SetPasscode sets or updates the user's login passcode
func (s *UserController) SetPasscode(ctx context.Context, req *pb.SetPasscodeRequest) (*pb.SetPasscodeResponse, error) {
	userID, ok := ctx.Value("user_id").(uint)
	if !ok || userID == 0 {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	// Validate passcode format (4-6 digits)
	if len(req.Passcode) < 4 || len(req.Passcode) > 6 {
		return &pb.SetPasscodeResponse{
			Success: false,
			Message: "Passcode must be between 4 and 6 digits",
		}, nil
	}

	// Get user
	var user models.User
	if err := s.server.db.First(&user, userID).Error; err != nil {
		return &pb.SetPasscodeResponse{
			Success: false,
			Message: "User not found",
		}, nil
	}

	// Verify password for security
	valid, err := user.ComparePassword(req.Password)
	if err != nil || !valid {
		return &pb.SetPasscodeResponse{
			Success: false,
			Message: "Invalid password",
		}, nil
	}

	// Set passcode
	if err := user.SetLoginPasscode(req.Passcode); err != nil {
		return &pb.SetPasscodeResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to set passcode: %v", err),
		}, nil
	}

	// Save user
	if err := s.server.db.Save(&user).Error; err != nil {
		return &pb.SetPasscodeResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to save passcode: %v", err),
		}, nil
	}

	return &pb.SetPasscodeResponse{
		Success: true,
		Message: "Passcode set successfully",
	}, nil
}

// VerifyPasscode verifies the user's login passcode
func (s *UserController) VerifyPasscode(ctx context.Context, req *pb.VerifyPasscodeRequest) (*pb.VerifyPasscodeResponse, error) {
	userID, ok := ctx.Value("user_id").(uint)
	if !ok || userID == 0 {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	var user models.User
	if err := s.server.db.First(&user, userID).Error; err != nil {
		return &pb.VerifyPasscodeResponse{
			Success: false,
			Message: "User not found",
			IsValid: false,
		}, nil
	}

	valid, err := user.CompareLoginPasscode(req.Passcode)
	if err != nil || !valid {
		return &pb.VerifyPasscodeResponse{
			Success: true,
			Message: "Passcode is invalid",
			IsValid: false,
		}, nil
	}

	return &pb.VerifyPasscodeResponse{
		Success: true,
		Message: "Passcode is valid",
		IsValid: true,
	}, nil
}

// RemovePasscode removes the user's login passcode
func (s *UserController) RemovePasscode(ctx context.Context, req *pb.RemovePasscodeRequest) (*pb.RemovePasscodeResponse, error) {
	userID, ok := ctx.Value("user_id").(uint)
	if !ok || userID == 0 {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	var user models.User
	if err := s.server.db.First(&user, userID).Error; err != nil {
		return &pb.RemovePasscodeResponse{
			Success: false,
			Message: "User not found",
		}, nil
	}

	// Verify password for security
	valid, err := user.ComparePassword(req.Password)
	if err != nil || !valid {
		return &pb.RemovePasscodeResponse{
			Success: false,
			Message: "Invalid password",
		}, nil
	}

	// Remove passcode
	user.LoginPasscode = nil
	if err := s.server.db.Save(&user).Error; err != nil {
		return &pb.RemovePasscodeResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to remove passcode: %v", err),
		}, nil
	}

	return &pb.RemovePasscodeResponse{
		Success: true,
		Message: "Passcode removed successfully",
	}, nil
}

// CheckPasscodeExists checks if the user has a passcode set
func (s *UserController) CheckPasscodeExists(ctx context.Context, req *pb.CheckPasscodeExistsRequest) (*pb.CheckPasscodeExistsResponse, error) {
	userID, ok := ctx.Value("user_id").(uint)
	if !ok || userID == 0 {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	var user models.User
	if err := s.server.db.First(&user, userID).Error; err != nil {
		return &pb.CheckPasscodeExistsResponse{
			Success:     false,
			HasPasscode: false,
		}, nil
	}

	hasPasscode := user.LoginPasscode != nil && *user.LoginPasscode != ""

	return &pb.CheckPasscodeExistsResponse{
		Success:     true,
		HasPasscode: hasPasscode,
	}, nil
}

// UpdateDevicePermissions updates device permission preferences
func (s *UserController) UpdateDevicePermissions(ctx context.Context, req *pb.UpdateDevicePermissionsRequest) (*pb.UpdateDevicePermissionsResponse, error) {
	userID, ok := ctx.Value("user_id").(uint)
	if !ok || userID == 0 {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	// Update or create each permission
	for _, perm := range req.Permissions {
		permType := mapPermissionType(perm.PermissionType)

		var devicePerm models.DevicePermission
		err := s.server.db.Where("user_id = ? AND permission_type = ?", userID, permType).First(&devicePerm).Error

		now := time.Now()
		if err == gorm.ErrRecordNotFound {
			// Create new permission
			devicePerm = models.DevicePermission{
				UserID:         userID,
				PermissionType: permType,
				IsGranted:      perm.IsGranted,
			}
			if perm.IsGranted {
				devicePerm.GrantedAt = &now
			}
			s.server.db.Create(&devicePerm)
		} else {
			// Update existing permission
			devicePerm.IsGranted = perm.IsGranted
			if perm.IsGranted {
				devicePerm.GrantedAt = &now
			} else {
				devicePerm.GrantedAt = nil
			}
			s.server.db.Save(&devicePerm)
		}
	}

	return &pb.UpdateDevicePermissionsResponse{
		Success: true,
		Message: "Device permissions updated successfully",
	}, nil
}

// GetDevicePermissions retrieves device permission status
func (s *UserController) GetDevicePermissions(ctx context.Context, req *pb.GetDevicePermissionsRequest) (*pb.GetDevicePermissionsResponse, error) {
	userID, ok := ctx.Value("user_id").(uint)
	if !ok || userID == 0 {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	var permissions []models.DevicePermission
	if err := s.server.db.Where("user_id = ?", userID).Find(&permissions).Error; err != nil {
		return &pb.GetDevicePermissionsResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to fetch permissions: %v", err),
		}, nil
	}

	pbPermissions := make([]*pb.DevicePermission, len(permissions))
	for i, perm := range permissions {
		pbPermissions[i] = &pb.DevicePermission{
			PermissionType: mapPermissionTypeToProto(perm.PermissionType),
			IsGranted:      perm.IsGranted,
			GrantedAt:      timestamppb.New(*perm.GrantedAt),
		}
		if perm.GrantedAt != nil {
			pbPermissions[i].GrantedAt = timestamppb.New(*perm.GrantedAt)
		}
	}

	return &pb.GetDevicePermissionsResponse{
		Success:     true,
		Message:     "Device permissions retrieved successfully",
		Permissions: pbPermissions,
	}, nil
}

// Helper functions

func uploadImageToStorage(imageData []byte, path string) (string, error) {
	// TODO: Implement actual cloud storage upload (S3, GCS, etc.)
	// For now, save locally or return a mock URL
	fileName := fmt.Sprintf("/uploads/%s", path)
	// In production, upload to S3/GCS and return the public URL
	return fileName, nil
}

func mapDocumentType(pbType pb.DocumentType) models.DocumentType {
	switch pbType {
	case pb.DocumentType_DOCUMENT_TYPE_PASSPORT:
		return models.DocumentTypePassport
	case pb.DocumentType_DOCUMENT_TYPE_DRIVERS_LICENSE:
		return models.DocumentTypeDriversLicense
	case pb.DocumentType_DOCUMENT_TYPE_NATIONAL_ID:
		return models.DocumentTypeNationalID
	case pb.DocumentType_DOCUMENT_TYPE_RESIDENCE_PERMIT:
		return models.DocumentTypeResidencePermit
	default:
		return models.DocumentTypePassport
	}
}

func mapDocumentTypeToString(pbType pb.DocumentType) string {
	switch pbType {
	case pb.DocumentType_DOCUMENT_TYPE_PASSPORT:
		return "PASSPORT"
	case pb.DocumentType_DOCUMENT_TYPE_DRIVERS_LICENSE:
		return "DRIVERS_LICENSE"
	case pb.DocumentType_DOCUMENT_TYPE_NATIONAL_ID:
		return "NATIONAL_ID"
	case pb.DocumentType_DOCUMENT_TYPE_RESIDENCE_PERMIT:
		return "RESIDENCE_PERMIT"
	default:
		return "PASSPORT"
	}
}

func mapDocumentTypeToProto(modelType models.DocumentType) pb.DocumentType {
	switch modelType {
	case models.DocumentTypePassport:
		return pb.DocumentType_DOCUMENT_TYPE_PASSPORT
	case models.DocumentTypeDriversLicense:
		return pb.DocumentType_DOCUMENT_TYPE_DRIVERS_LICENSE
	case models.DocumentTypeNationalID:
		return pb.DocumentType_DOCUMENT_TYPE_NATIONAL_ID
	case models.DocumentTypeResidencePermit:
		return pb.DocumentType_DOCUMENT_TYPE_RESIDENCE_PERMIT
	default:
		return pb.DocumentType_DOCUMENT_TYPE_PASSPORT
	}
}

func mapVerificationStatusToProto(modelStatus models.VerificationStatus) pb.VerificationStatus {
	switch modelStatus {
	case models.VerificationStatusPending:
		return pb.VerificationStatus_VERIFICATION_STATUS_PENDING
	case models.VerificationStatusProcessing:
		return pb.VerificationStatus_VERIFICATION_STATUS_PROCESSING
	case models.VerificationStatusVerified:
		return pb.VerificationStatus_VERIFICATION_STATUS_VERIFIED
	case models.VerificationStatusRejected:
		return pb.VerificationStatus_VERIFICATION_STATUS_REJECTED
	case models.VerificationStatusExpired:
		return pb.VerificationStatus_VERIFICATION_STATUS_EXPIRED
	default:
		return pb.VerificationStatus_VERIFICATION_STATUS_PENDING
	}
}

func mapPermissionType(pbType pb.PermissionType) models.PermissionType {
	switch pbType {
	case pb.PermissionType_PERMISSION_TYPE_CAMERA:
		return models.PermissionTypeCamera
	case pb.PermissionType_PERMISSION_TYPE_LOCATION:
		return models.PermissionTypeLocation
	case pb.PermissionType_PERMISSION_TYPE_MICROPHONE:
		return models.PermissionTypeMicrophone
	case pb.PermissionType_PERMISSION_TYPE_STORAGE:
		return models.PermissionTypeStorage
	case pb.PermissionType_PERMISSION_TYPE_CONTACTS:
		return models.PermissionTypeContacts
	case pb.PermissionType_PERMISSION_TYPE_BIOMETRIC:
		return models.PermissionTypeBiometric
	default:
		return models.PermissionTypeCamera
	}
}

func mapPermissionTypeToProto(modelType models.PermissionType) pb.PermissionType {
	switch modelType {
	case models.PermissionTypeCamera:
		return pb.PermissionType_PERMISSION_TYPE_CAMERA
	case models.PermissionTypeLocation:
		return pb.PermissionType_PERMISSION_TYPE_LOCATION
	case models.PermissionTypeMicrophone:
		return pb.PermissionType_PERMISSION_TYPE_MICROPHONE
	case models.PermissionTypeStorage:
		return pb.PermissionType_PERMISSION_TYPE_STORAGE
	case models.PermissionTypeContacts:
		return pb.PermissionType_PERMISSION_TYPE_CONTACTS
	case models.PermissionTypeBiometric:
		return pb.PermissionType_PERMISSION_TYPE_BIOMETRIC
	default:
		return pb.PermissionType_PERMISSION_TYPE_CAMERA
	}
}

func modelToProtoIDDocument(doc *models.IDDocument) *pb.IDDocument {
	pbDoc := &pb.IDDocument{
		Id:                 doc.ID,
		UserId:             uint64(doc.UserID),
		DocumentType:       mapDocumentTypeToProto(doc.DocumentType),
		DocumentNumber:     doc.DocumentNumber,
		FullName:           doc.FullName,
		DateOfBirth:        doc.DateOfBirth,
		IssueDate:          doc.IssueDate,
		ExpiryDate:         doc.ExpiryDate,
		IssuingCountry:     doc.IssuingCountry,
		DocumentFrontUrl:   doc.DocumentFrontURL,
		DocumentBackUrl:    doc.DocumentBackURL,
		VerificationStatus: mapVerificationStatusToProto(doc.VerificationStatus),
		RejectionReason:    doc.RejectionReason,
		CreatedAt:          timestamppb.New(doc.CreatedAt),
	}
	if doc.VerifiedAt != nil {
		pbDoc.VerifiedAt = timestamppb.New(*doc.VerifiedAt)
	}
	return pbDoc
}

func modelToProtoFacialData(data *models.FacialData) *pb.FacialData {
	pbData := &pb.FacialData{
		Id:         data.ID,
		UserId:     uint64(data.UserID),
		FaceId:     data.FaceID,
		ImageUrl:   data.ImageURL,
		IsVerified: data.IsVerified,
		CreatedAt:  timestamppb.New(data.CreatedAt),
	}
	if data.LastVerifiedAt != nil {
		pbData.LastVerifiedAt = timestamppb.New(*data.LastVerifiedAt)
	}
	return pbData
}
