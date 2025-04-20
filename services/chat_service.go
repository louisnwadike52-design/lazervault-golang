package services

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"lazervaultGo/models"
	"lazervaultGo/pb"

	"gorm.io/gorm"
)

const DefaultPageSize = 50

var (
	ErrInvalidUserID      = errors.New("invalid sender or receiver user ID")
	ErrMessageNotFound    = errors.New("message not found")
	ErrReplyToMsgNotFound = errors.New("reply-to message not found")
	ErrInvalidMessageType = errors.New("invalid message type")
	ErrContentRequired    = errors.New("message content is required")
)

// subscriber represents a single client subscribed to chat updates
type subscriber struct {
	id      string                     // Unique ID for the subscriber
	msgChan chan<- *models.ChatMessage // Channel to send messages to the client
}

// IChatService defines the interface for chat operations
type IChatService interface {
	SendMessage(ctx context.Context, req *SendMessageServiceRequest) (*models.ChatMessage, error)
	GetChatHistory(ctx context.Context, req *GetChatHistoryServiceRequest) ([]models.ChatMessage, string, error)
	StreamChatMessages(ctx context.Context, req *StreamChatHistoryServiceRequest) (<-chan *models.ChatMessage, func(), error)
}

// ChatService implements the IChatService interface
type ChatService struct {
	db            *gorm.DB
	mu            sync.RWMutex             // Mutex to protect subscriptions map
	subscriptions map[string][]*subscriber // Map from chat_identifier -> list of subscribers
}

// NewChatService creates a new ChatService
func NewChatService(db *gorm.DB) IChatService {
	return &ChatService{
		db:            db,
		subscriptions: make(map[string][]*subscriber),
	}
}

// Helper to generate a unique, consistent identifier for a chat between two users
func getChatIdentifier(userID1, userID2 string) string {
	ids := []string{userID1, userID2}
	sort.Strings(ids) // Ensure order doesn't matter
	return strings.Join(ids, "_")
}

// SendMessageServiceRequest contains parameters for sending a message
type SendMessageServiceRequest struct {
	SenderUserID     string
	ReceiverUserID   string
	Content          string
	MessageType      pb.MessageType // Use the proto enum
	AttachmentURL    *string        // Optional
	ReplyToMessageID *string        // Optional
}

// SendMessage handles creating, storing, and broadcasting a new chat message
func (s *ChatService) SendMessage(ctx context.Context, req *SendMessageServiceRequest) (*models.ChatMessage, error) {
	if req.SenderUserID == "" || req.ReceiverUserID == "" {
		return nil, ErrInvalidUserID
	}
	if req.Content == "" {
		return nil, ErrContentRequired
	}

	// Basic validation for message type
	if _, ok := pb.MessageType_name[int32(req.MessageType)]; !ok {
		return nil, ErrInvalidMessageType
	}

	// Validate attachment URL presence for non-text types (optional strictness)
	if req.MessageType != pb.MessageType_TEXT && (req.AttachmentURL == nil || *req.AttachmentURL == "") {
		// Depending on requirements, you might enforce this or allow it
		// return nil, fmt.Errorf("attachment URL is required for %s messages", req.MessageType.String())
	}

	// Check if replying to a valid message (if ReplyToMessageID is provided)
	if req.ReplyToMessageID != nil && *req.ReplyToMessageID != "" {
		var repliedMsg models.ChatMessage
		if err := s.db.WithContext(ctx).First(&repliedMsg, "id = ?", *req.ReplyToMessageID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrReplyToMsgNotFound
			}
			return nil, fmt.Errorf("database error checking reply message: %w", err)
		}
		// Optional: Check if the replied message belongs to the same chat conversation
		if !((repliedMsg.SenderUserID == req.SenderUserID && repliedMsg.ReceiverUserID == req.ReceiverUserID) ||
			(repliedMsg.SenderUserID == req.ReceiverUserID && repliedMsg.ReceiverUserID == req.SenderUserID)) {
			return nil, fmt.Errorf("reply message does not belong to this chat")
		}
	}

	// Create the message model
	message := &models.ChatMessage{
		SenderUserID:     req.SenderUserID,
		ReceiverUserID:   req.ReceiverUserID,
		Content:          req.Content,
		MessageType:      req.MessageType.String(), // Store as string
		AttachmentURL:    req.AttachmentURL,
		ReplyToMessageID: req.ReplyToMessageID,
		Timestamp:        time.Now().UTC(),
	}

	// Save to database
	if err := s.db.WithContext(ctx).Create(message).Error; err != nil {
		return nil, fmt.Errorf("failed to save message: %w", err)
	}

	// Broadcast the message to active subscribers for this chat
	s.broadcastMessage(message)

	return message, nil
}

// broadcastMessage sends a message to all subscribed clients for that chat
func (s *ChatService) broadcastMessage(message *models.ChatMessage) {
	chatId := getChatIdentifier(message.SenderUserID, message.ReceiverUserID)

	s.mu.RLock() // Read lock to access subscriptions
	subs, ok := s.subscriptions[chatId]
	s.mu.RUnlock()

	if !ok {
		return // No one is currently listening to this chat
	}

	s.mu.RLock() // RLock again for the duration of sending
	defer s.mu.RUnlock()

	// Send non-blockingly to avoid one slow client blocking others
	// TODO: Consider buffer sizes or alternative strategies for slow clients
	for _, sub := range subs {
		go func(s *subscriber) { // Send in a goroutine
			select {
			case s.msgChan <- message:
				// Message sent
			case <-time.After(1 * time.Second): // Timeout to prevent blocking forever
				fmt.Printf("Warning: timeout sending message to subscriber %s for chat %s\n", s.id, chatId)
				// Optionally, consider removing the subscriber if it times out frequently
			}
		}(sub)
	}
}

// GetChatHistoryServiceRequest contains parameters for fetching chat history
type GetChatHistoryServiceRequest struct {
	UserID1   string // Authenticated user
	UserID2   string // Peer user
	PageSize  int
	PageToken string // Using timestamp as page token for simplicity (e.g., "<timestamp_unix_nano>")
}

// GetChatHistory retrieves messages between two users with pagination
func (s *ChatService) GetChatHistory(ctx context.Context, req *GetChatHistoryServiceRequest) ([]models.ChatMessage, string, error) {
	if req.UserID1 == "" || req.UserID2 == "" {
		return nil, "", ErrInvalidUserID
	}

	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = DefaultPageSize
	}

	var messages []models.ChatMessage
	query := s.db.WithContext(ctx).Model(&models.ChatMessage{}).
		Where("(sender_user_id = ? AND receiver_user_id = ?) OR (sender_user_id = ? AND receiver_user_id = ?)",
			req.UserID1, req.UserID2, req.UserID2, req.UserID1)

	// Pagination based on timestamp (older messages first)
	if req.PageToken != "" {
		// Assume page token is the ID of the last message of the previous page
		// Need to find that message first to get its timestamp for keyset pagination
		var lastMsg models.ChatMessage
		err := s.db.WithContext(ctx).Select("timestamp").First(&lastMsg, "id = ?", req.PageToken).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				// Invalid token, maybe return empty or error
				return []models.ChatMessage{}, "", fmt.Errorf("invalid page token: %w", err)
			}
			return nil, "", fmt.Errorf("failed to query page token message: %w", err)
		}
		// Fetch messages older than the message identified by the token
		// Using (timestamp, id) for more robust keyset pagination
		query = query.Where("(timestamp, id) < (?, ?)", lastMsg.Timestamp, req.PageToken)

		// Simple timestamp only pagination (less robust):
		// var tokenTime time.Time
		// Placeholder: For now, we ignore page_token and just limit
		// A real implementation would parse the token and add a WHERE clause like:
		// query = query.Where("timestamp < ?", tokenTime)
	}

	// Order by timestamp descending (most recent first) and limit
	// Add +1 to page size to check if there's a next page
	err := query.Order("timestamp desc, id desc").Limit(pageSize + 1).Find(&messages).Error // Add secondary sort by ID
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, "", fmt.Errorf("failed to retrieve chat history: %w", err)
	}

	// Determine next page token
	nextPageToken := ""
	if len(messages) > pageSize {
		// There are more messages, set the token based on the last message retrieved (which is the pageSize-th item)
		nextPageToken = messages[pageSize-1].ID // Use the ID of the last message in the current page
		messages = messages[:pageSize]          // Trim the extra message used for check
	}

	// Reverse the messages slice so the oldest message is first in the result (optional, depends on desired API output)
	// for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
	// 	messages[i], messages[j] = messages[j], messages[i]
	// }

	return messages, nextPageToken, nil
}

// StreamChatHistoryServiceRequest contains parameters for streaming messages
type StreamChatHistoryServiceRequest struct {
	UserID1 string // Authenticated user
	UserID2 string // Peer user
}

// StreamChatMessages subscribes a client to real-time updates for a chat
// It returns a channel to receive messages and a function to unsubscribe.
func (s *ChatService) StreamChatMessages(ctx context.Context, req *StreamChatHistoryServiceRequest) (<-chan *models.ChatMessage, func(), error) {
	if req.UserID1 == "" || req.UserID2 == "" {
		return nil, nil, ErrInvalidUserID
	}

	// TODO: Add authorization check if needed (e.g., ensure UserID1 matches authenticated user)

	chatId := getChatIdentifier(req.UserID1, req.UserID2)
	clientChan := make(chan *models.ChatMessage, 10) // Buffered channel

	sub := &subscriber{
		id:      fmt.Sprintf("%s_%d", chatId, time.Now().UnixNano()), // Simple unique ID
		msgChan: clientChan,
	}

	s.mu.Lock() // Write lock to modify subscriptions
	s.subscriptions[chatId] = append(s.subscriptions[chatId], sub)
	s.mu.Unlock()

	fmt.Printf("Subscriber %s added for chat %s\n", sub.id, chatId)

	// Unsubscribe function to be called when the client disconnects
	unsubscribe := func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		currentSubs := s.subscriptions[chatId]
		newSubs := make([]*subscriber, 0, len(currentSubs)-1)
		found := false
		for _, s := range currentSubs {
			if s.id != sub.id {
				newSubs = append(newSubs, s)
			} else {
				found = true
			}
		}

		if found {
			if len(newSubs) > 0 {
				s.subscriptions[chatId] = newSubs
			} else {
				delete(s.subscriptions, chatId) // Clean up map if no subscribers left
			}
			close(sub.msgChan) // Close the channel to signal controller
			fmt.Printf("Subscriber %s removed for chat %s\n", sub.id, chatId)
		} else {
			fmt.Printf("Warning: Subscriber %s not found for removal in chat %s\n", sub.id, chatId)
		}
	}

	// Monitor context cancellation in a goroutine to trigger unsubscribe
	go func() {
		<-ctx.Done() // Wait for client disconnection (context cancellation)
		unsubscribe()
	}()

	return clientChan, unsubscribe, nil
}
