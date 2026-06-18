package proxy

import (
	"context"

	whatsapppb "whatsapp-service/proto"
)

// WhatsAppServiceProxy proxies WhatsAppService gRPC requests to the whatsapp-service
type WhatsAppServiceProxy struct {
	whatsapppb.UnimplementedWhatsAppServiceServer
	client whatsapppb.WhatsAppServiceClient
}

// NewWhatsAppServiceProxy creates a new WhatsAppServiceProxy
func NewWhatsAppServiceProxy(client whatsapppb.WhatsAppServiceClient) *WhatsAppServiceProxy {
	return &WhatsAppServiceProxy{
		client: client,
	}
}

// InitiateLinking proxies to whatsapp-service
func (p *WhatsAppServiceProxy) InitiateLinking(ctx context.Context, req *whatsapppb.InitiateLinkingRequest) (*whatsapppb.InitiateLinkingResponse, error) {
	return p.client.InitiateLinking(forwardContext(ctx), req)
}

// VerifyLinking proxies to whatsapp-service
func (p *WhatsAppServiceProxy) VerifyLinking(ctx context.Context, req *whatsapppb.VerifyLinkingRequest) (*whatsapppb.VerifyLinkingResponse, error) {
	return p.client.VerifyLinking(forwardContext(ctx), req)
}

// UnlinkAccount proxies to whatsapp-service
func (p *WhatsAppServiceProxy) UnlinkAccount(ctx context.Context, req *whatsapppb.UnlinkAccountRequest) (*whatsapppb.UnlinkAccountResponse, error) {
	return p.client.UnlinkAccount(forwardContext(ctx), req)
}

// GetLinkStatus proxies to whatsapp-service
func (p *WhatsAppServiceProxy) GetLinkStatus(ctx context.Context, req *whatsapppb.GetLinkStatusRequest) (*whatsapppb.GetLinkStatusResponse, error) {
	return p.client.GetLinkStatus(forwardContext(ctx), req)
}

// HandleWebhook proxies to whatsapp-service
func (p *WhatsAppServiceProxy) HandleWebhook(ctx context.Context, req *whatsapppb.WebhookRequest) (*whatsapppb.WebhookResponse, error) {
	return p.client.HandleWebhook(forwardContext(ctx), req)
}

// VerifyWebhook proxies to whatsapp-service
func (p *WhatsAppServiceProxy) VerifyWebhook(ctx context.Context, req *whatsapppb.VerifyWebhookRequest) (*whatsapppb.VerifyWebhookResponse, error) {
	return p.client.VerifyWebhook(forwardContext(ctx), req)
}

// GetSession proxies to whatsapp-service
func (p *WhatsAppServiceProxy) GetSession(ctx context.Context, req *whatsapppb.GetSessionRequest) (*whatsapppb.GetSessionResponse, error) {
	return p.client.GetSession(forwardContext(ctx), req)
}

// InvalidateSession proxies to whatsapp-service
func (p *WhatsAppServiceProxy) InvalidateSession(ctx context.Context, req *whatsapppb.InvalidateSessionRequest) (*whatsapppb.InvalidateSessionResponse, error) {
	return p.client.InvalidateSession(forwardContext(ctx), req)
}

// UpdateSecuritySettings proxies to whatsapp-service
func (p *WhatsAppServiceProxy) UpdateSecuritySettings(ctx context.Context, req *whatsapppb.UpdateSecuritySettingsRequest) (*whatsapppb.UpdateSecuritySettingsResponse, error) {
	return p.client.UpdateSecuritySettings(forwardContext(ctx), req)
}

// GetSecuritySettings proxies to whatsapp-service
func (p *WhatsAppServiceProxy) GetSecuritySettings(ctx context.Context, req *whatsapppb.GetSecuritySettingsRequest) (*whatsapppb.GetSecuritySettingsResponse, error) {
	return p.client.GetSecuritySettings(forwardContext(ctx), req)
}

// GetAuditLogs proxies to whatsapp-service
func (p *WhatsAppServiceProxy) GetAuditLogs(ctx context.Context, req *whatsapppb.GetAuditLogsRequest) (*whatsapppb.GetAuditLogsResponse, error) {
	return p.client.GetAuditLogs(forwardContext(ctx), req)
}
