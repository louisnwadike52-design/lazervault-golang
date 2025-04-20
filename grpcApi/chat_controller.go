package grpcApi

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"lazervaultGo/grpcApi/middleware"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"lazervaultGo/services"
	"lazervaultGo/token"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ChatController handles gRPC requests for chat operations
type ChatController struct {
	pb.UnimplementedChatServiceServer // Embed for forward compatibility
	chatService                       services.IChatService
	// db needed? If service handles all DB interaction, maybe not.
	// tokenMaker token.Maker // If needed for direct token validation
}

// NewChatController creates a new ChatController
func NewChatController(chatService services.IChatService) *ChatController {
	return &ChatController{
		chatService: chatService,
	}
}

// Helper to convert models.ChatMessage to pb.ChatMessage
func convertChatMessageToProto(msg *models.ChatMessage) *pb.ChatMessage {
	if msg == nil {
		return nil
	}
	msgType, _ := pb.MessageType_value[msg.MessageType] // Convert string back to enum value

	pbMsg := &pb.ChatMessage{
		MessageId:      msg.ID,
		SenderUserId:   msg.SenderUserID,
		ReceiverUserId: msg.ReceiverUserID,
		Content:        msg.Content,
		Timestamp:      timestamppb.New(msg.Timestamp),
		MessageType:    pb.MessageType(msgType),
	}
	if msg.AttachmentURL != nil {
		pbMsg.AttachmentUrl = *msg.AttachmentURL
	}
	if msg.ReplyToMessageID != nil {
		pbMsg.ReplyToMessageId = *msg.ReplyToMessageID
	}
	return pbMsg
}

// SendMessage handles the gRPC request to send a message
func (controller *ChatController) SendMessage(ctx context.Context, req *pb.SendMessageRequest) (*pb.SendMessageResponse, error) {
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "unable to retrieve payload from context")
	}

	// Validate basic request fields
	if req.GetReceiverUserId() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "receiver_user_id is required")
	}
	if req.GetContent() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "content is required")
	}
	if _, ok := pb.MessageType_name[int32(req.GetMessageType())]; !ok {
		return nil, status.Errorf(codes.InvalidArgument, "invalid message_type")
	}

	senderIdentifier := authPayload.Email // Placeholder: Using email from token

	serviceReq := &services.SendMessageServiceRequest{
		SenderUserID:   senderIdentifier, // Use the identifier derived from token
		ReceiverUserID: req.GetReceiverUserId(),
		Content:        req.GetContent(),
		MessageType:    req.GetMessageType(),
	}

	if req.AttachmentUrl != "" {
		serviceReq.AttachmentURL = &req.AttachmentUrl
	}
	if req.ReplyToMessageId != "" {
		serviceReq.ReplyToMessageID = &req.ReplyToMessageId
	}

	createdMsg, err := controller.chatService.SendMessage(ctx, serviceReq)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrInvalidUserID):
			return nil, status.Errorf(codes.InvalidArgument, "invalid user ID: %v", err)
		case errors.Is(err, services.ErrContentRequired):
			return nil, status.Errorf(codes.InvalidArgument, "content required: %v", err)
		case errors.Is(err, services.ErrInvalidMessageType):
			return nil, status.Errorf(codes.InvalidArgument, "invalid message type: %v", err)
		case errors.Is(err, services.ErrReplyToMsgNotFound):
			return nil, status.Errorf(codes.NotFound, "reply message not found: %v", err)
		default:
			// Check for specific DB errors if needed
			return nil, status.Errorf(codes.Internal, "failed to send message: %v", err)
		}
	}

	resp := &pb.SendMessageResponse{
		Message: convertChatMessageToProto(createdMsg),
	}

	return resp, nil
}

// GetChatHistory handles the gRPC request to retrieve chat history
func (controller *ChatController) GetChatHistory(ctx context.Context, req *pb.GetChatHistoryRequest) (*pb.GetChatHistoryResponse, error) {
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "unable to retrieve payload from context")
	}

	if req.GetPeerUserId() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "peer_user_id is required")
	}

	user1Identifier := authPayload.Email // Placeholder: Using email from token

	serviceReq := &services.GetChatHistoryServiceRequest{
		UserID1:   user1Identifier, // Use the identifier derived from token
		UserID2:   req.GetPeerUserId(),
		PageSize:  int(req.GetPageSize()), // Handle potential overflow if needed
		PageToken: req.GetPageToken(),
	}

	messages, nextToken, err := controller.chatService.GetChatHistory(ctx, serviceReq)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrInvalidUserID):
			return nil, status.Errorf(codes.InvalidArgument, "invalid user ID: %v", err)
		// Handle specific pagination errors if service returns them
		case errors.Is(err, sql.ErrNoRows):
			// Not an error, just no messages
			return &pb.GetChatHistoryResponse{Messages: []*pb.ChatMessage{}, NextPageToken: ""}, nil
		default:
			return nil, status.Errorf(codes.Internal, "failed to get chat history: %v", err)
		}
	}

	pbMessages := make([]*pb.ChatMessage, len(messages))
	for i, msg := range messages {
		pbMessages[i] = convertChatMessageToProto(&msg)
	}

	resp := &pb.GetChatHistoryResponse{
		Messages:      pbMessages,
		NextPageToken: nextToken,
	}

	return resp, nil
}

// StreamChatMessages handles the server-streaming RPC for real-time messages
func (controller *ChatController) StreamChatMessages(req *pb.StreamChatHistoryRequest, stream pb.ChatService_StreamChatMessagesServer) error {
	// Get authenticated user info from context
	authPayload, ok := stream.Context().Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok {
		return status.Errorf(codes.Unauthenticated, "unable to retrieve payload from context")
	}

	if req.GetPeerUserId() == "" {
		return status.Errorf(codes.InvalidArgument, "peer_user_id is required")
	}

	// TODO: Fetch UserID1 from DB based on authPayload.Email
	user1Identifier := authPayload.Email // Placeholder

	serviceReq := &services.StreamChatHistoryServiceRequest{
		UserID1: user1Identifier,
		UserID2: req.GetPeerUserId(),
	}

	// Subscribe to the message stream from the service
	// The context passed here is stream.Context(), so when the client disconnects,
	// the service's goroutine monitoring ctx.Done() will trigger the unsubscribe.
	msgChan, unsubscribe, err := controller.chatService.StreamChatMessages(stream.Context(), serviceReq)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrInvalidUserID):
			return status.Errorf(codes.InvalidArgument, "invalid user ID: %v", err)
		// Handle other specific errors from service if needed
		default:
			fmt.Printf("Error subscribing to chat stream: %v\n", err) // Log error
			return status.Errorf(codes.Internal, "failed to start chat stream")
		}
	}
	// Ensure unsubscribe is called if this function returns prematurely
	// Although the context cancellation should handle it, this adds robustness.
	defer unsubscribe()

	fmt.Printf("User %s started streaming chat with %s\n", user1Identifier, req.GetPeerUserId())

	// Loop and forward messages from the service channel to the gRPC stream
	for {
		select {
		case <-stream.Context().Done():
			// Client disconnected
			fmt.Printf("Client disconnected from chat stream (%s <-> %s)\n", user1Identifier, req.GetPeerUserId())
			return nil // Clean disconnect
		case msg, ok := <-msgChan:
			if !ok {
				// Channel closed by the service (e.g., during shutdown or explicit unsubscribe)
				fmt.Printf("Chat stream channel closed (%s <-> %s)\n", user1Identifier, req.GetPeerUserId())
				return nil
			}

			// Convert message to proto and send to client
			pbMsg := convertChatMessageToProto(msg)
			if err := stream.Send(pbMsg); err != nil {
				// Error sending to client stream
				fmt.Printf("Error sending message to client stream (%s <-> %s): %v\n", user1Identifier, req.GetPeerUserId(), err)
				// Returning an error will terminate the stream
				// Consider specific error codes (e.g., codes.Unavailable)
				return status.Errorf(codes.Internal, "error sending message to stream: %v", err)
			}
		}
	}
}
