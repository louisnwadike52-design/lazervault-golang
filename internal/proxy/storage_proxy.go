// storage_proxy.go — small Gin handler that lets the Flutter app obtain a
// scoped upload URL from storage-service WITHOUT being trusted to talk to
// storage-service directly (storage-service requires X-Service-Name once
// STORAGE_ALLOWED_SERVICES is set in prod).
//
// Endpoints:
//
//   POST /api/v1/profile-picture/upload-url   (JWT-protected)
//     body: { "filename": "selfie.jpg", "content_type": "image/jpeg" }
//     resp: { "upload_url": "...", "public_url": "...", "key": "...", "expires_at": "..." }
//
//     Storage key locked server-side to:
//       users/<user_id>/profile-<uuid>.<ext>
//
//   POST /api/v1/bank-scan/upload-url         (JWT-protected)
//     body: { "filename": "scan.jpg", "content_type": "image/jpeg" }
//     resp: { "upload_url": "...", "public_url": "...", "key": "...", "expires_at": "..." }
//
//     Storage key locked server-side to:
//       users/<user_id>/bank-scans/<uuid>.<ext>
//
// Same allow-list of image MIMEs (jpg/png/webp/heic/gif) — bank-scan
// reuses the profile-picture pipeline because the threat surface is
// identical (single-user-scoped image upload destined for downstream
// vision OCR / display). The Flutter app does the PUT itself (same
// pattern as the insurance document upload — see create_policy_cubit.dart
// `_putBytes`) and then posts the resulting `public_url` to
// chat-agent-gateway `/scan/bank-details`.
package proxy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// StorageProxy holds the storage-service base URL + a shared HTTP client.
type StorageProxy struct {
	storageBaseURL string
	serviceName    string
	// publicBaseURL is the externally-reachable storage origin (e.g.
	// "https://dev.lazervault.app"). The storage-service stamps INTERNAL urls
	// (http://localhost:8094/...) into upload/public URLs — fine for server-side
	// callers, but a phone uploading via the tunnel cannot reach localhost. When set
	// (STORAGE_PUBLIC_URL), we rewrite the scheme+host of the client-facing upload_url
	// / public_url to this origin so device uploads work. Empty = no rewrite.
	publicBaseURL string
	httpClient    *http.Client
}

// NewStorageProxy builds the proxy. `storageBaseURL` must be the storage-
// service root WITHOUT a trailing slash (e.g. "http://localhost:8091" or
// "https://storage.lazervault.app"). The upload-url path is appended.
func NewStorageProxy(storageBaseURL, serviceName string) *StorageProxy {
	return &StorageProxy{
		storageBaseURL: strings.TrimRight(storageBaseURL, "/"),
		serviceName:    serviceName,
		publicBaseURL:  strings.TrimRight(os.Getenv("STORAGE_PUBLIC_URL"), "/"),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// allowedImageContentTypes restricts profile pictures to common image
// MIME types. Anything else is rejected before we even hit storage.
var allowedImageContentTypes = map[string]string{
	"image/jpeg": "jpg",
	"image/jpg":  "jpg",
	"image/png":  "png",
	"image/webp": "webp",
	"image/heic": "heic",
	"image/gif":  "gif",
}

// allowedImageExts is the parallel allow-list used when the client sends
// a filename but no Content-Type we recognise.
var allowedImageExts = map[string]string{
	"jpg":  "image/jpeg",
	"jpeg": "image/jpeg",
	"png":  "image/png",
	"webp": "image/webp",
	"heic": "image/heic",
	"gif":  "image/gif",
}

// allowedChatMediaContentTypes extends the image allow-list with the audio
// MIME types used for P2P chat voice notes (record plugin emits aac-in-m4a).
var allowedChatMediaContentTypes = map[string]string{
	"image/jpeg":  "jpg",
	"image/jpg":   "jpg",
	"image/png":   "png",
	"image/webp":  "webp",
	"image/heic":  "heic",
	"image/gif":   "gif",
	"audio/mp4":   "m4a",
	"audio/m4a":   "m4a",
	"audio/aac":   "aac",
	"audio/mpeg":  "mp3",
	"audio/x-m4a": "m4a",
}

// allowedChatMediaExts is the parallel filename-based allow-list for chat media.
var allowedChatMediaExts = map[string]string{
	"jpg":  "image/jpeg",
	"jpeg": "image/jpeg",
	"png":  "image/png",
	"webp": "image/webp",
	"heic": "image/heic",
	"gif":  "image/gif",
	"m4a":  "audio/mp4",
	"aac":  "audio/aac",
	"mp3":  "audio/mpeg",
}

// allowedEscrowMediaContentTypes extends the image allow-list with the short-clip
// video MIME types escrow deals accept as evidence (product photo/video, delivery
// proof, dispute/refund evidence). Video is capped small (see escrowVideoMaxBytes)
// so a one-minute clip never fills storage.
var allowedEscrowMediaContentTypes = map[string]string{
	"image/jpeg":      "jpg",
	"image/jpg":       "jpg",
	"image/png":       "png",
	"image/webp":      "webp",
	"image/heic":      "heic",
	"image/gif":       "gif",
	"video/mp4":       "mp4",
	"video/quicktime": "mov",
	"video/webm":      "webm",
}

// allowedEscrowMediaExts is the parallel filename-based allow-list for escrow media.
var allowedEscrowMediaExts = map[string]string{
	"jpg":  "image/jpeg",
	"jpeg": "image/jpeg",
	"png":  "image/png",
	"webp": "image/webp",
	"heic": "image/heic",
	"gif":  "image/gif",
	"mp4":  "video/mp4",
	"mov":  "video/quicktime",
	"webm": "video/webm",
}

// Escrow evidence size caps (client enforces + asks to compress first; these are
// surfaced to the client and kept in sync with escrow-service + storage-service).
const (
	escrowImageMaxBytes = 8 * 1024 * 1024  // 8 MB
	escrowVideoMaxBytes = 10 * 1024 * 1024 // 10 MB
)

type uploadURLClientRequest struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
}

// Backwards-compatible alias — previous name was specific to profile
// pictures, but the request shape is shared by every signed-upload
// endpoint we proxy. Kept so existing callers that pass this type
// remain compatible if they grew an external dependency in the
// meantime.
type profilePictureUploadURLRequest = uploadURLClientRequest

type storageUploadURLRequest struct {
	Service     string `json:"service"`
	UserID      string `json:"user_id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Key         string `json:"key,omitempty"`
}

type storageUploadURLResponse struct {
	UploadURL string `json:"upload_url"`
	PublicURL string `json:"public_url"`
	Key       string `json:"key"`
	ExpiresAt string `json:"expires_at"`
}

// HandleProfilePictureUploadURL handles POST /api/v1/profile-picture/upload-url.
func (p *StorageProxy) HandleProfilePictureUploadURL(c *gin.Context) {
	p.handleScopedUploadURL(c, "profile")
}

// HandleBankScanUploadURL handles POST /api/v1/bank-scan/upload-url. Same
// shape as the profile-picture handler, different keyspace so a future
// reconciler / audit query can scope to "bank-scan images for user X"
// without grepping over their profile photos.
func (p *StorageProxy) HandleBankScanUploadURL(c *gin.Context) {
	p.handleScopedUploadURL(c, "bank-scan")
}

// HandleChatMediaUploadURL handles POST /api/v1/chat-media/upload-url. Same
// JWT-scoped shape as profile-picture/bank-scan, but the chat-media keyspace
// also permits audio MIME types (voice notes) in addition to images.
func (p *StorageProxy) HandleChatMediaUploadURL(c *gin.Context) {
	p.handleScopedUploadURL(c, "chat-media")
}

// HandleInvoiceUploadURL handles POST /api/v1/invoice/upload-url. Same
// JWT-scoped shape as profile-picture; the invoice keyspace holds the issuer /
// customer logos shown on an invoice so they can be scoped/audited separately.
func (p *StorageProxy) HandleInvoiceUploadURL(c *gin.Context) {
	p.handleScopedUploadURL(c, "invoice")
}

// HandleEscrowUploadURL handles POST /api/v1/escrow/upload-url. Holds the
// buyer's product/service image and the seller's proof-of-delivery image for
// an escrow deal, scoped to the uploading user.
func (p *StorageProxy) HandleEscrowUploadURL(c *gin.Context) {
	p.handleScopedUploadURL(c, "escrow")
}

// HandleFCYDocumentUploadURL handles POST /api/v1/fcy-document/upload-url.
// Holds the KYC documents a foreign-currency virtual-account request
// requires (government ID, utility bill / bank statement) — images or PDF,
// scoped to the uploading user; the resulting public URL is what the
// accounts-service FCY request forwards to Fincra for compliance review.
func (p *StorageProxy) HandleFCYDocumentUploadURL(c *gin.Context) {
	p.handleScopedUploadURL(c, "fcy-document")
}

// handleScopedUploadURL is the shared implementation for every per-user
// image-upload endpoint we proxy. `keyspace` selects which sub-prefix
// inside `users/<user_id>/` the generated object key lives under.
// Supported keyspaces (extend here, not by adding a parallel handler):
//   - "profile":   users/<user_id>/profile-<uuid>.<ext>
//   - "bank-scan": users/<user_id>/bank-scans/<uuid>.<ext>
func (p *StorageProxy) handleScopedUploadURL(c *gin.Context, keyspace string) {
	userIDAny, _ := c.Get("user_id")
	userID, _ := userIDAny.(string)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "user_not_authenticated",
			"message": "missing user_id from JWT",
		})
		return
	}

	var req uploadURLClientRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid_request",
			"message": err.Error(),
		})
		return
	}

	var (
		ext         string
		contentType string
		typeErr     error
	)
	switch keyspace {
	case "chat-media":
		ext, contentType, typeErr = resolveChatMediaTypeAndExt(req.Filename, req.ContentType)
	case "escrow":
		ext, contentType, typeErr = resolveEscrowMediaTypeAndExt(req.Filename, req.ContentType)
	case "fcy-document":
		ext, contentType, typeErr = resolveFCYDocumentTypeAndExt(req.Filename, req.ContentType)
	default:
		ext, contentType, typeErr = resolveImageTypeAndExt(req.Filename, req.ContentType)
	}
	if typeErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "unsupported_media_type",
			"message": typeErr.Error(),
		})
		return
	}

	key, defaultName, err := buildScopedKey(keyspace, userID, ext)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "invalid_keyspace",
			"message": err.Error(),
		})
		return
	}

	upstream := storageUploadURLRequest{
		Service:     p.serviceName,
		UserID:      userID,
		Filename:    safeFilenameWithDefault(req.Filename, defaultName),
		ContentType: contentType,
		Key:         key,
	}
	body, err := json.Marshal(upstream)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "marshal_failed",
			"message": err.Error(),
		})
		return
	}

	url := p.storageBaseURL + "/v1/storage/upload-url"
	httpReq, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "upstream_request_build_failed",
			"message": err.Error(),
		})
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Service-Name", p.serviceName)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"error":   "storage_unreachable",
			"message": err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.JSON(http.StatusBadGateway, gin.H{
			"success":         false,
			"error":           "storage_rejected",
			"message":         string(respBody),
			"upstream_status": resp.StatusCode,
		})
		return
	}

	var parsed storageUploadURLResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"error":   "storage_response_parse_failed",
			"message": err.Error(),
		})
		return
	}

	mediaKind := "image"
	maxBytes := escrowImageMaxBytes
	if strings.HasPrefix(contentType, "video/") {
		mediaKind = "video"
		maxBytes = escrowVideoMaxBytes
	}
	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"upload_url":   p.toPublicURL(parsed.UploadURL),
		"public_url":   p.toPublicURL(parsed.PublicURL),
		"key":          parsed.Key,
		"expires_at":   parsed.ExpiresAt,
		"content_type": contentType,
		"media_kind":   mediaKind,
		"max_bytes":    maxBytes,
	})
}

// resolveEscrowMediaTypeAndExt is the escrow counterpart of resolveImageTypeAndExt —
// it accepts images AND the short-clip video MIME types escrow deals allow.
func resolveEscrowMediaTypeAndExt(filename, contentType string) (string, string, error) {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if ct != "" {
		if ext, ok := allowedEscrowMediaContentTypes[ct]; ok {
			return ext, ct, nil
		}
	}
	if filename != "" {
		ext := strings.TrimPrefix(strings.ToLower(path.Ext(filename)), ".")
		if mt, ok := allowedEscrowMediaExts[ext]; ok {
			return ext, mt, nil
		}
	}
	return "", "", errors.New("filename or content_type must identify a supported image (jpg, png, webp, heic, gif) or video (mp4, mov, webm)")
}

// toPublicURL rewrites an internal storage URL's scheme+host to the externally
// reachable origin (STORAGE_PUBLIC_URL), so a phone uploading via the tunnel can
// actually reach it. The path (/v1/storage/objects/...) is preserved — the tunnel
// routes that to the storage-service. No-op when STORAGE_PUBLIC_URL is unset or the
// input is unparseable.
func (p *StorageProxy) toPublicURL(u string) string {
	if p.publicBaseURL == "" || u == "" {
		return u
	}
	parsed, err := neturl.Parse(u)
	if err != nil {
		return u
	}
	pub, err := neturl.Parse(p.publicBaseURL)
	if err != nil || pub.Host == "" {
		return u
	}
	parsed.Scheme = pub.Scheme
	parsed.Host = pub.Host
	return parsed.String()
}

// buildScopedKey returns the storage key + default filename for a given
// keyspace. Server-side keying means the client cannot influence the
// destination — they can only target their own scoped prefix.
func buildScopedKey(keyspace, userID, ext string) (string, string, error) {
	switch keyspace {
	case "profile":
		return fmt.Sprintf("users/%s/profile-%s.%s", userID, uuid.NewString(), ext),
			"profile." + ext, nil
	case "bank-scan":
		return fmt.Sprintf("users/%s/bank-scans/%s.%s", userID, uuid.NewString(), ext),
			"bank-scan." + ext, nil
	case "chat-media":
		return fmt.Sprintf("users/%s/chat-media/%s.%s", userID, uuid.NewString(), ext),
			"chat-media." + ext, nil
	case "invoice":
		return fmt.Sprintf("users/%s/invoices/%s.%s", userID, uuid.NewString(), ext),
			"invoice." + ext, nil
	case "escrow":
		return fmt.Sprintf("users/%s/escrow/%s.%s", userID, uuid.NewString(), ext),
			"escrow." + ext, nil
	case "fcy-document":
		return fmt.Sprintf("users/%s/fcy-documents/%s.%s", userID, uuid.NewString(), ext),
			"fcy-document." + ext, nil
	default:
		return "", "", errors.New("unknown keyspace: " + keyspace)
	}
}

// resolveFCYDocumentTypeAndExt allows the document formats Fincra's FCY
// compliance review accepts as URLs (their examples are PDFs; scans/photos
// of IDs and utility bills are images): images + application/pdf.
func resolveFCYDocumentTypeAndExt(filename, contentType string) (string, string, error) {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if ct == "application/pdf" {
		return "pdf", "application/pdf", nil
	}
	if strings.HasSuffix(strings.ToLower(filename), ".pdf") {
		return "pdf", "application/pdf", nil
	}
	return resolveImageTypeAndExt(filename, contentType)
}

// resolveImageTypeAndExt decides on the canonical Content-Type + file
// extension for the upload, given whatever combo the client sent. The
// allow-list keeps non-image junk out of the profile-picture keyspace.
func resolveImageTypeAndExt(filename, contentType string) (string, string, error) {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if ct != "" {
		if ext, ok := allowedImageContentTypes[ct]; ok {
			return ext, ct, nil
		}
	}
	if filename != "" {
		ext := strings.TrimPrefix(strings.ToLower(path.Ext(filename)), ".")
		if mt, ok := allowedImageExts[ext]; ok {
			return ext, mt, nil
		}
	}
	return "", "", errors.New("filename or content_type must identify a supported image (jpg, png, webp, heic, gif)")
}

// resolveChatMediaTypeAndExt is the chat-media counterpart of
// resolveImageTypeAndExt — it accepts images AND audio voice-note MIME types.
func resolveChatMediaTypeAndExt(filename, contentType string) (string, string, error) {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if ct != "" {
		if ext, ok := allowedChatMediaContentTypes[ct]; ok {
			return ext, ct, nil
		}
	}
	if filename != "" {
		ext := strings.TrimPrefix(strings.ToLower(path.Ext(filename)), ".")
		if mt, ok := allowedChatMediaExts[ext]; ok {
			return ext, mt, nil
		}
	}
	return "", "", errors.New("filename or content_type must identify a supported image or audio file")
}

// safeFilenameWithDefault returns a sanitised filename for the upstream
// request, falling back to `defaultName` when the client-supplied name
// is empty or a path-only artefact. storage-service uses our `Key`
// field anyway; the filename only matters for logging on the storage
// side, so we don't need to be picky beyond stripping directory
// components.
func safeFilenameWithDefault(name, defaultName string) string {
	base := strings.TrimSpace(name)
	if base == "" {
		return defaultName
	}
	base = path.Base(base)
	if base == "." || base == "/" {
		return defaultName
	}
	return base
}
