package proxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// ClientLogsProxy ingests structured LOG batches from the Flutter app and pushes
// them into our internal Loki (via /loki/api/v1/push), integrating device logs
// with the backend logs promtail already ships. This keeps Loki PRIVATE — the
// app never touches Loki directly (platform rule: the frontend never calls a
// backend/infra component directly). The route is registered BEFORE the JWT
// middleware so pre-login screens (the biometric lock) can also ship logs; the
// user_id in the payload is a LABEL-ONLY field (never an auth decision), so we
// don't need to verify a token here.
//
// Loki cardinality hygiene: only a small, bounded label set goes into stream
// labels; everything high-cardinality (user_id, device_id, session_id, message,
// request id, arbitrary fields) lives INSIDE the log line JSON.
type ClientLogsProxy struct {
	pushURL     string
	client      *http.Client
	enabled     bool
	sampleRate  float64
	maxEvents   int
	maxBodySize int64
}

// NewClientLogsProxy builds the proxy. LOKI_PUSH_URL defaults to the in-cluster
// Loki push endpoint. CLIENT_LOGS_ENABLED / CLIENT_LOGS_SAMPLE_RATE let ops flip
// the ingest off or throttle it WITHOUT an app release (the values are echoed
// back to the client, which self-adjusts).
func NewClientLogsProxy() *ClientLogsProxy {
	push := os.Getenv("LOKI_PUSH_URL")
	if push == "" {
		push = "http://loki:3100/loki/api/v1/push"
	}
	enabled := strings.ToLower(strings.TrimSpace(os.Getenv("CLIENT_LOGS_ENABLED"))) != "false"
	rate := 1.0
	if v := strings.TrimSpace(os.Getenv("CLIENT_LOGS_SAMPLE_RATE")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 && f <= 1 {
			rate = f
		}
	}
	return &ClientLogsProxy{
		pushURL:     push,
		client:      &http.Client{Timeout: 8 * time.Second},
		enabled:     enabled,
		sampleRate:  rate,
		maxEvents:   500,
		maxBodySize: 1 << 20, // 1 MiB per batch
	}
}

type clientLogEvent struct {
	Ts     string                 `json:"ts"`
	Level  string                 `json:"level"`
	Flow   string                 `json:"flow"`
	Msg    string                 `json:"msg"`
	Screen string                 `json:"screen,omitempty"`
	Fields map[string]interface{} `json:"fields,omitempty"`
}

type clientLogBatch struct {
	DeviceID   string           `json:"device_id"`
	SessionID  string           `json:"session_id"`
	AppVersion string           `json:"app_version"`
	AppEnv     string           `json:"app_env"`
	Platform   string           `json:"platform"`
	OSVersion  string           `json:"os_version"`
	UserID     string           `json:"user_id"`
	Events     []clientLogEvent `json:"events"`
}

// lokiPush is the JSON shape Loki's push API expects.
type lokiPush struct {
	Streams []lokiStream `json:"streams"`
}
type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][2]string       `json:"values"`
}

// Handle parses one batch, translates to Loki streams keyed by a low-cardinality
// label set, and pushes. Best-effort and defensive — a malformed batch is
// dropped with 4xx; Loki being down returns 202 anyway (we never want the app
// retrying a logging call aggressively). Always echoes the current
// enabled/sample_rate so the client can self-throttle.
func (p *ClientLogsProxy) Handle(c *gin.Context) {
	// Server kill-switch: accept + drop so the client stops sending.
	if !p.enabled {
		c.JSON(http.StatusAccepted, gin.H{"accepted": 0, "enabled": false, "sample_rate": 0})
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, p.maxBodySize)
	var batch clientLogBatch
	if err := json.NewDecoder(c.Request.Body).Decode(&batch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid batch"})
		return
	}
	if len(batch.Events) == 0 {
		c.JSON(http.StatusAccepted, gin.H{"accepted": 0, "enabled": true, "sample_rate": p.sampleRate})
		return
	}
	if len(batch.Events) > p.maxEvents {
		batch.Events = batch.Events[:p.maxEvents] // clamp; never trust client size
	}

	appEnv := sanitizeLabel(firstNonEmpty(batch.AppEnv, os.Getenv("ENVIRONMENT"), "unknown"))
	platform := sanitizeLabel(firstNonEmpty(batch.Platform, "unknown"))
	appVersion := sanitizeLabel(firstNonEmpty(batch.AppVersion, "unknown"))
	requestID := c.GetHeader("X-Request-ID")

	// Group values by the label set so Loki gets few, stable streams.
	streams := map[string]*lokiStream{}
	now := time.Now()
	for _, e := range batch.Events {
		level := sanitizeLabel(normalizeLevel(e.Level))
		flow := sanitizeLabel(firstNonEmpty(e.Flow, "app"))
		labels := map[string]string{
			"source":      "flutter",
			"app_env":     appEnv,
			"platform":    platform,
			"app_version": appVersion,
			"level":       level,
			"flow":        flow,
		}
		key := level + "|" + flow // app_env/platform/version constant per batch

		// The full log line: high-cardinality context lives here, not in labels.
		line := map[string]interface{}{
			"msg":         e.Msg,
			"level":       level,
			"flow":        flow,
			"device_id":   batch.DeviceID,
			"session_id":  batch.SessionID,
			"os_version":  batch.OSVersion,
			"app_version": batch.AppVersion,
		}
		if batch.UserID != "" {
			line["user_id"] = batch.UserID
		}
		if e.Screen != "" {
			line["screen"] = e.Screen
		}
		if requestID != "" {
			line["request_id"] = requestID
		}
		for k, v := range e.Fields {
			// Don't let a field clobber a reserved key.
			if _, taken := line[k]; !taken {
				line[k] = v
			}
		}
		lineJSON, _ := json.Marshal(line)

		st, ok := streams[key]
		if !ok {
			st = &lokiStream{Stream: labels, Values: [][2]string{}}
			streams[key] = st
		}
		st.Values = append(st.Values, [2]string{clampTsNanos(e.Ts, now), string(lineJSON)})
	}

	payload := lokiPush{Streams: make([]lokiStream, 0, len(streams))}
	for _, st := range streams {
		payload.Streams = append(payload.Streams, *st)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		c.JSON(http.StatusAccepted, gin.H{"accepted": 0, "enabled": true, "sample_rate": p.sampleRate})
		return
	}

	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, p.pushURL, bytes.NewReader(body))
	if err == nil {
		req.Header.Set("Content-Type", "application/json")
		resp, perr := p.client.Do(req)
		if perr != nil {
			// Loki unreachable — accept anyway so the app doesn't hot-retry.
			log.Warn().Err(perr).Msg("client-logs: loki push failed")
		} else {
			resp.Body.Close()
			if resp.StatusCode >= 300 {
				log.Warn().Int("status", resp.StatusCode).Msg("client-logs: loki push non-2xx")
			}
		}
	}

	c.JSON(http.StatusAccepted, gin.H{
		"accepted":    len(batch.Events),
		"enabled":     true,
		"sample_rate": p.sampleRate,
	})
}

// clampTsNanos converts a client RFC3339 timestamp to Loki nanosecond string,
// clamped to [now-24h, now+5m] so a wrong device clock can't poison the stream.
func clampTsNanos(ts string, now time.Time) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		t = now
	}
	if t.Before(now.Add(-24 * time.Hour)) {
		t = now.Add(-24 * time.Hour)
	}
	if t.After(now.Add(5 * time.Minute)) {
		t = now
	}
	return strconv.FormatInt(t.UnixNano(), 10)
}

func normalizeLevel(l string) string {
	switch strings.ToLower(strings.TrimSpace(l)) {
	case "error", "severe", "fatal":
		return "error"
	case "warn", "warning":
		return "warn"
	case "debug", "fine":
		return "debug"
	default:
		return "info"
	}
}

// sanitizeLabel keeps Loki labels safe + low-cardinality: lower-case, bounded,
// restricted charset.
func sanitizeLabel(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "unknown"
	}
	if len(s) > 48 {
		s = s[:48]
	}
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' || r == '+' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
