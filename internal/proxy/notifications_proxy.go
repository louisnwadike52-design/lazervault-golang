package proxy

import (
	"context"

	notificationspb "notifications-service/proto"
)

type NotificationsServiceProxy struct {
	notificationspb.UnimplementedNotificationsServiceServer
	client notificationspb.NotificationsServiceClient
}

func NewNotificationsServiceProxy(client notificationspb.NotificationsServiceClient) *NotificationsServiceProxy {
	return &NotificationsServiceProxy{client: client}
}

func (p *NotificationsServiceProxy) RegisterFCMToken(ctx context.Context, req *notificationspb.RegisterFCMTokenRequest) (*notificationspb.RegisterFCMTokenResponse, error) {
	return p.client.RegisterFCMToken(forwardContext(ctx), req)
}

func (p *NotificationsServiceProxy) GetNotifications(ctx context.Context, req *notificationspb.GetNotificationsRequest) (*notificationspb.GetNotificationsResponse, error) {
	return p.client.GetNotifications(forwardContext(ctx), req)
}

func (p *NotificationsServiceProxy) MarkAsRead(ctx context.Context, req *notificationspb.MarkAsReadRequest) (*notificationspb.MarkAsReadResponse, error) {
	return p.client.MarkAsRead(forwardContext(ctx), req)
}

func (p *NotificationsServiceProxy) MarkAllAsRead(ctx context.Context, req *notificationspb.MarkAllAsReadRequest) (*notificationspb.MarkAllAsReadResponse, error) {
	return p.client.MarkAllAsRead(forwardContext(ctx), req)
}

func (p *NotificationsServiceProxy) DeleteNotification(ctx context.Context, req *notificationspb.DeleteNotificationRequest) (*notificationspb.DeleteNotificationResponse, error) {
	return p.client.DeleteNotification(forwardContext(ctx), req)
}

func (p *NotificationsServiceProxy) GetNotificationPreferences(ctx context.Context, req *notificationspb.GetNotificationPreferencesRequest) (*notificationspb.GetNotificationPreferencesResponse, error) {
	return p.client.GetNotificationPreferences(forwardContext(ctx), req)
}

func (p *NotificationsServiceProxy) UpdateNotificationPreferences(ctx context.Context, req *notificationspb.UpdateNotificationPreferencesRequest) (*notificationspb.UpdateNotificationPreferencesResponse, error) {
	return p.client.UpdateNotificationPreferences(forwardContext(ctx), req)
}

func (p *NotificationsServiceProxy) SendTestNotification(ctx context.Context, req *notificationspb.SendTestNotificationRequest) (*notificationspb.SendTestNotificationResponse, error) {
	return p.client.SendTestNotification(forwardContext(ctx), req)
}
