package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

// --- Service Errors ---
var (
	ErrContactSyncFailed         = errors.New("contact sync service: failed to sync contacts")
	ErrContactNotFound           = errors.New("contact sync service: contact not found")
	ErrContactDeleteFailed       = errors.New("contact sync service: failed to delete contacts")
	ErrPreferencesUpdateFailed   = errors.New("contact sync service: failed to update preferences")
	ErrRecipientConversionFailed = errors.New("contact sync service: failed to convert contact to recipient")
	ErrInvalidSyncFrequency      = errors.New("contact sync service: invalid sync frequency")
)

// --- Service Interface ---
type IContactSyncService interface {
	SyncContacts(ctx context.Context, userID uint, contactsToSync []*pb.ContactToSync, replaceAll bool) (*SyncContactsResult, error)
	GetSyncedContacts(ctx context.Context, userID uint, page, pageSize int32, searchQuery string, onlyLazerVaultUsers bool) (*GetSyncedContactsResult, error)
	DeleteSyncedContacts(ctx context.Context, userID uint, contactIDs []string, deleteAll bool) (int, error)
	ConvertContactToRecipient(ctx context.Context, userID uint, req *pb.ConvertContactToRecipientRequest) (*ConvertContactResult, error)
	FindLazerVaultUsers(ctx context.Context, phoneNumbers []string, emails []string) ([]*LazerVaultUserMatch, error)
	UpdateSyncPreferences(ctx context.Context, userID uint, req *pb.UpdateSyncPreferencesRequest) (*models.SyncPreferences, error)
}

// --- Service Struct ---
type ContactSyncService struct {
	db *gorm.DB
}

// --- Constructor ---
func NewContactSyncService(db *gorm.DB) IContactSyncService {
	return &ContactSyncService{db: db}
}

// --- Result Types ---
type SyncContactsResult struct {
	SyncedContacts    []*models.SyncedContact
	TotalSynced       int
	TotalMatchedUsers int
	MatchedUsers      []*LazerVaultUserMatch
}

type GetSyncedContactsResult struct {
	Contacts   []*models.SyncedContact
	TotalCount int64
	Page       int32
	PageSize   int32
}

type ConvertContactResult struct {
	RecipientID        uint
	IsLazerVaultUser   bool
	LazerVaultUserID   *uint
	LazerVaultUsername string
	Recipient          *models.Recipient
}

type LazerVaultUserMatch struct {
	ContactID       string
	UserID          uint
	Username        string
	Name            string
	ProfilePhotoURL string
	IsVerified      bool
	MatchedBy       string // "phone" or "email"
}

// --- Methods ---

// SyncContacts syncs device contacts to the backend and matches with LazerVault users
func (s *ContactSyncService) SyncContacts(ctx context.Context, userID uint, contactsToSync []*pb.ContactToSync, replaceAll bool) (*SyncContactsResult, error) {
	tx := s.db.WithContext(ctx).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// If replaceAll is true, delete existing synced contacts
	if replaceAll {
		if err := tx.Where("user_id = ?", userID).Delete(&models.SyncedContact{}).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("%w: %v", ErrContactSyncFailed, err)
		}
	}

	syncedContacts := make([]*models.SyncedContact, 0, len(contactsToSync))
	now := time.Now()

	// Collect all phone numbers and emails for batch matching
	allPhones := make([]string, 0)
	allEmails := make([]string, 0)
	for _, contact := range contactsToSync {
		allPhones = append(allPhones, contact.PhoneNumbers...)
		allEmails = append(allEmails, contact.Emails...)
	}

	// Find matching LazerVault users
	userMatches, err := s.findUserMatches(tx, allPhones, allEmails)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("%w: failed to match users: %v", ErrContactSyncFailed, err)
	}

	// Create a map for quick lookup
	phoneUserMap := make(map[string]*models.User)
	emailUserMap := make(map[string]*models.User)
	for phone, user := range userMatches.phoneMatches {
		phoneUserMap[phone] = user
	}
	for email, user := range userMatches.emailMatches {
		emailUserMap[email] = user
	}

	// Sync each contact
	for _, contactProto := range contactsToSync {
		// Marshal phone numbers and emails to JSON
		phoneJSON, _ := json.Marshal(contactProto.PhoneNumbers)
		emailJSON, _ := json.Marshal(contactProto.Emails)

		// Check if this contact matches a LazerVault user
		var matchedUser *models.User
		for _, phone := range contactProto.PhoneNumbers {
			if user, ok := phoneUserMap[phone]; ok {
				matchedUser = user
				break
			}
		}
		if matchedUser == nil {
			for _, email := range contactProto.Emails {
				if user, ok := emailUserMap[email]; ok {
					matchedUser = user
					break
				}
			}
		}

		syncedContact := models.SyncedContact{
			UserID:          userID,
			Name:            contactProto.Name,
			PhoneNumbers:    string(phoneJSON),
			Emails:          string(emailJSON),
			DeviceContactID: contactProto.DeviceContactId,
			LastSyncedAt:    now,
		}

		if matchedUser != nil {
			syncedContact.IsLazerVaultUser = true
			syncedContact.LazerVaultUserID = &matchedUser.ID
		}

		// Upsert: Update if exists (by device_contact_id + user_id), create if not
		var existing models.SyncedContact
		err := tx.Where("user_id = ? AND device_contact_id = ?", userID, contactProto.DeviceContactId).
			First(&existing).Error

		if err == nil {
			// Update existing
			existing.Name = syncedContact.Name
			existing.PhoneNumbers = syncedContact.PhoneNumbers
			existing.Emails = syncedContact.Emails
			existing.LastSyncedAt = syncedContact.LastSyncedAt
			existing.IsLazerVaultUser = syncedContact.IsLazerVaultUser
			existing.LazerVaultUserID = syncedContact.LazerVaultUserID

			if err := tx.Save(&existing).Error; err != nil {
				tx.Rollback()
				return nil, fmt.Errorf("%w: %v", ErrContactSyncFailed, err)
			}
			syncedContacts = append(syncedContacts, &existing)
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			// Create new
			if err := tx.Create(&syncedContact).Error; err != nil {
				tx.Rollback()
				return nil, fmt.Errorf("%w: %v", ErrContactSyncFailed, err)
			}
			syncedContacts = append(syncedContacts, &syncedContact)
		} else {
			tx.Rollback()
			return nil, fmt.Errorf("%w: %v", ErrContactSyncFailed, err)
		}
	}

	// Update sync preferences
	var prefs models.SyncPreferences
	err = tx.Where("user_id = ?", userID).First(&prefs).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Create default preferences
		prefs = models.SyncPreferences{
			UserID:              userID,
			AutoSyncEnabled:     false,
			SyncFrequency:       models.SyncFrequencyManual,
			MatchWithUsers:      true,
			SyncPhotos:          false,
			TotalSyncedContacts: len(syncedContacts),
			TotalMatchedUsers:   len(userMatches.allMatches),
		}
		now := time.Now()
		prefs.LastSyncAt = &now
		if err := tx.Create(&prefs).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("%w: failed to create preferences: %v", ErrContactSyncFailed, err)
		}
	} else if err == nil {
		// Update existing preferences
		now := time.Now()
		prefs.LastSyncAt = &now
		var totalCount int64
		tx.Model(&models.SyncedContact{}).Where("user_id = ?", userID).Count(&totalCount)
		prefs.TotalSyncedContacts = int(totalCount)
		prefs.TotalMatchedUsers = len(userMatches.allMatches)
		if err := tx.Save(&prefs).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("%w: failed to update preferences: %v", ErrContactSyncFailed, err)
		}
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("%w: %v", ErrContactSyncFailed, err)
	}

	// Build matched users result
	matchedUsersResult := make([]*LazerVaultUserMatch, 0, len(userMatches.allMatches))
	for _, match := range userMatches.allMatches {
		matchedUsersResult = append(matchedUsersResult, match)
	}

	return &SyncContactsResult{
		SyncedContacts:    syncedContacts,
		TotalSynced:       len(syncedContacts),
		TotalMatchedUsers: len(matchedUsersResult),
		MatchedUsers:      matchedUsersResult,
	}, nil
}

// GetSyncedContacts retrieves synced contacts with pagination and filtering
func (s *ContactSyncService) GetSyncedContacts(ctx context.Context, userID uint, page, pageSize int32, searchQuery string, onlyLazerVaultUsers bool) (*GetSyncedContactsResult, error) {
	var contacts []*models.SyncedContact
	query := s.db.WithContext(ctx).Where("user_id = ?", userID)

	// Apply search filter
	if searchQuery != "" {
		searchTerm := "%" + strings.ToLower(searchQuery) + "%"
		query = query.Where("LOWER(name) LIKE ?", searchTerm)
	}

	// Filter for LazerVault users only
	if onlyLazerVaultUsers {
		query = query.Where("is_lazervault_user = ?", true)
	}

	// Get total count
	var totalCount int64
	if err := query.Model(&models.SyncedContact{}).Count(&totalCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count contacts: %w", err)
	}

	// Apply pagination
	offset := page * pageSize
	if err := query.Order("name ASC").Limit(int(pageSize)).Offset(int(offset)).Find(&contacts).Error; err != nil {
		return nil, fmt.Errorf("failed to retrieve contacts: %w", err)
	}

	return &GetSyncedContactsResult{
		Contacts:   contacts,
		TotalCount: totalCount,
		Page:       page,
		PageSize:   pageSize,
	}, nil
}

// DeleteSyncedContacts deletes synced contacts
func (s *ContactSyncService) DeleteSyncedContacts(ctx context.Context, userID uint, contactIDs []string, deleteAll bool) (int, error) {
	var result *gorm.DB

	if deleteAll {
		result = s.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&models.SyncedContact{})
	} else if len(contactIDs) > 0 {
		result = s.db.WithContext(ctx).Where("user_id = ? AND id IN ?", userID, contactIDs).Delete(&models.SyncedContact{})
	} else {
		return 0, errors.New("must provide contact IDs or set deleteAll to true")
	}

	if result.Error != nil {
		return 0, fmt.Errorf("%w: %v", ErrContactDeleteFailed, result.Error)
	}

	return int(result.RowsAffected), nil
}

// ConvertContactToRecipient converts a synced contact to a saved recipient
func (s *ContactSyncService) ConvertContactToRecipient(ctx context.Context, userID uint, req *pb.ConvertContactToRecipientRequest) (*ConvertContactResult, error) {
	tx := s.db.WithContext(ctx).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var matchedUser *models.User
	isLazerVaultUser := false

	// Auto-detect LazerVault user if requested
	if req.AutoDetectLazervault {
		phones := []string{}
		emails := []string{}
		if req.PhoneNumber != "" {
			phones = []string{req.PhoneNumber}
		}
		if req.Email != "" {
			emails = []string{req.Email}
		}

		matches, err := s.findUserMatches(tx, phones, emails)
		if err == nil && len(matches.allMatches) > 0 {
			// Take the first match
			for _, match := range matches.allMatches {
				var user models.User
				if err := tx.First(&user, match.UserID).Error; err == nil {
					matchedUser = &user
					isLazerVaultUser = true
					break
				}
			}
		}
	}

	// Create recipient
	recipient := models.Recipient{
		OwnerUserID: userID,
		Name:        req.Name,
		IsFavorite:  false,
		Type:        "external",
	}

	if isLazerVaultUser && matchedUser != nil {
		// Create internal recipient
		recipient.Type = "internal"

		// Find the user's account
		var account models.Account
		if err := tx.Where("owner_user_id = ?", matchedUser.ID).First(&account).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("%w: failed to find user account: %v", ErrRecipientConversionFailed, err)
		}

		accountID := account.ID
		recipient.InternalAccountID = &accountID
		recipient.InternalUserID = &matchedUser.ID
	} else {
		// Create external recipient
		recipient.AccountNumber = req.AccountNumber
		recipient.BankName = req.BankName
		recipient.SortCode = req.SortCode
	}

	if err := tx.Create(&recipient).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("%w: %v", ErrRecipientConversionFailed, err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRecipientConversionFailed, err)
	}

	result := &ConvertContactResult{
		RecipientID:      recipient.ID,
		IsLazerVaultUser: isLazerVaultUser,
		Recipient:        &recipient,
	}

	if matchedUser != nil {
		result.LazerVaultUserID = &matchedUser.ID
		result.LazerVaultUsername = matchedUser.Email // You might want to use a username field if available
	}

	return result, nil
}

// FindLazerVaultUsers finds LazerVault users matching the provided phone numbers and emails
func (s *ContactSyncService) FindLazerVaultUsers(ctx context.Context, phoneNumbers []string, emails []string) ([]*LazerVaultUserMatch, error) {
	matches, err := s.findUserMatches(s.db.WithContext(ctx), phoneNumbers, emails)
	if err != nil {
		return nil, err
	}

	result := make([]*LazerVaultUserMatch, 0, len(matches.allMatches))
	for _, match := range matches.allMatches {
		result = append(result, match)
	}

	return result, nil
}

// UpdateSyncPreferences updates user's contact sync preferences
func (s *ContactSyncService) UpdateSyncPreferences(ctx context.Context, userID uint, req *pb.UpdateSyncPreferencesRequest) (*models.SyncPreferences, error) {
	var prefs models.SyncPreferences
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&prefs).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Create new preferences
		prefs = models.SyncPreferences{
			UserID:          userID,
			AutoSyncEnabled: req.AutoSyncEnabled,
			MatchWithUsers:  req.MatchWithUsers,
			SyncPhotos:      req.SyncPhotos,
		}

		// Convert proto frequency to model frequency
		prefs.SyncFrequency = convertProtoFrequencyToModel(req.SyncFrequency)

		if err := s.db.WithContext(ctx).Create(&prefs).Error; err != nil {
			return nil, fmt.Errorf("%w: %v", ErrPreferencesUpdateFailed, err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("failed to find preferences: %w", err)
	} else {
		// Update existing preferences
		prefs.AutoSyncEnabled = req.AutoSyncEnabled
		prefs.SyncFrequency = convertProtoFrequencyToModel(req.SyncFrequency)
		prefs.MatchWithUsers = req.MatchWithUsers
		prefs.SyncPhotos = req.SyncPhotos

		if err := s.db.WithContext(ctx).Save(&prefs).Error; err != nil {
			return nil, fmt.Errorf("%w: %v", ErrPreferencesUpdateFailed, err)
		}
	}

	return &prefs, nil
}

// --- Helper Types and Methods ---

type userMatchesResult struct {
	phoneMatches map[string]*models.User
	emailMatches map[string]*models.User
	allMatches   map[uint]*LazerVaultUserMatch
}

// findUserMatches implements the contact matching algorithm
func (s *ContactSyncService) findUserMatches(db *gorm.DB, phoneNumbers []string, emails []string) (*userMatchesResult, error) {
	result := &userMatchesResult{
		phoneMatches: make(map[string]*models.User),
		emailMatches: make(map[string]*models.User),
		allMatches:   make(map[uint]*LazerVaultUserMatch),
	}

	// Match by phone numbers
	if len(phoneNumbers) > 0 {
		var users []models.User
		if err := db.Where("phone_number IN ?", phoneNumbers).Find(&users).Error; err != nil {
			return nil, fmt.Errorf("failed to match phone numbers: %w", err)
		}

		for i := range users {
			user := &users[i]
			result.phoneMatches[user.PhoneNumber] = user
			result.allMatches[user.ID] = &LazerVaultUserMatch{
				UserID:     user.ID,
				Username:   user.Email, // Use email as username for now
				Name:       fmt.Sprintf("%s %s", user.FirstName, user.LastName),
				IsVerified: user.Verified,
				MatchedBy:  "phone",
			}
		}
	}

	// Match by emails
	if len(emails) > 0 {
		var users []models.User
		if err := db.Where("email IN ?", emails).Find(&users).Error; err != nil {
			return nil, fmt.Errorf("failed to match emails: %w", err)
		}

		for i := range users {
			user := &users[i]
			result.emailMatches[user.Email] = user
			// Only add to allMatches if not already matched by phone
			if _, exists := result.allMatches[user.ID]; !exists {
				result.allMatches[user.ID] = &LazerVaultUserMatch{
					UserID:     user.ID,
					Username:   user.Email,
					Name:       fmt.Sprintf("%s %s", user.FirstName, user.LastName),
					IsVerified: user.Verified,
					MatchedBy:  "email",
				}
			}
		}
	}

	return result, nil
}

// --- Conversion Helpers ---

func convertProtoFrequencyToModel(protoFreq pb.SyncFrequency) models.SyncFrequency {
	switch protoFreq {
	case pb.SyncFrequency_MANUAL:
		return models.SyncFrequencyManual
	case pb.SyncFrequency_DAILY:
		return models.SyncFrequencyDaily
	case pb.SyncFrequency_WEEKLY:
		return models.SyncFrequencyWeekly
	case pb.SyncFrequency_REAL_TIME:
		return models.SyncFrequencyRealTime
	default:
		return models.SyncFrequencyManual
	}
}

func convertModelFrequencyToProto(modelFreq models.SyncFrequency) pb.SyncFrequency {
	switch modelFreq {
	case models.SyncFrequencyManual:
		return pb.SyncFrequency_MANUAL
	case models.SyncFrequencyDaily:
		return pb.SyncFrequency_DAILY
	case models.SyncFrequencyWeekly:
		return pb.SyncFrequency_WEEKLY
	case models.SyncFrequencyRealTime:
		return pb.SyncFrequency_REAL_TIME
	default:
		return pb.SyncFrequency_SYNC_FREQUENCY_UNSPECIFIED
	}
}

// ConvertSyncedContactToProto converts a SyncedContact model to proto message
func ConvertSyncedContactToProto(c *models.SyncedContact) *pb.SyncedContact {
	if c == nil {
		return nil
	}

	protoContact := &pb.SyncedContact{
		Id:               fmt.Sprint(c.ID),
		UserId:           fmt.Sprint(c.UserID),
		Name:             c.Name,
		PhotoUrl:         c.PhotoURL,
		DeviceContactId:  c.DeviceContactID,
		IsLazervaultUser: c.IsLazerVaultUser,
		CreatedAt:        timestamppb.New(c.CreatedAt),
		UpdatedAt:        timestamppb.New(c.UpdatedAt),
	}

	// Unmarshal phone numbers
	var phones []string
	if err := json.Unmarshal([]byte(c.PhoneNumbers), &phones); err == nil {
		protoContact.PhoneNumbers = phones
	}

	// Unmarshal emails
	var emails []string
	if err := json.Unmarshal([]byte(c.Emails), &emails); err == nil {
		protoContact.Emails = emails
	}

	// Set LazerVault user info if applicable
	if c.LazerVaultUserID != nil {
		protoContact.LazervaultUserId = fmt.Sprint(*c.LazerVaultUserID)
		// Note: You might want to preload the User relationship to get the username
		// For now, we'll leave it empty and populate it in the controller if needed
	}

	return protoContact
}

// ConvertSyncPreferencesToProto converts SyncPreferences model to proto message
func ConvertSyncPreferencesToProto(p *models.SyncPreferences) *pb.SyncPreferences {
	if p == nil {
		return nil
	}

	protoPrefs := &pb.SyncPreferences{
		UserId:              fmt.Sprint(p.UserID),
		AutoSyncEnabled:     p.AutoSyncEnabled,
		SyncFrequency:       convertModelFrequencyToProto(p.SyncFrequency),
		MatchWithUsers:      p.MatchWithUsers,
		SyncPhotos:          p.SyncPhotos,
		TotalSyncedContacts: int32(p.TotalSyncedContacts),
		TotalMatchedUsers:   int32(p.TotalMatchedUsers),
	}

	if p.LastSyncAt != nil {
		protoPrefs.LastSyncAt = timestamppb.New(*p.LastSyncAt)
	}

	return protoPrefs
}
