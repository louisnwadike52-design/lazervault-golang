# Complete gRPC Request Format

This document shows the complete gRPC request format that the LazerVault Golang service expects for the voice-note endpoint.

## 📡 gRPC Service Definition

**Service**: `VoiceSessionService`  
**Method**: `ProcessVoiceNote`  
**Endpoint**: `POST /v1/voice/note/process`

## 📋 Complete Request Structure

### ProcessVoiceNoteRequest

```protobuf
message ProcessVoiceNoteRequest {
    bytes audio_content = 1;    // Required: Binary audio content (max 25MB)
    string filename = 2;        // Optional: Original filename
    string content_type = 3;    // Optional: MIME type
    string tx_history = 4;      // Optional: JSON string of transactions
}
```

### Go Struct Definition

```go
type ProcessVoiceNoteRequest struct {
    AudioContent []byte `json:"audio_content,omitempty"`
    Filename     string `json:"filename,omitempty"`
    ContentType  string `json:"content_type,omitempty"`
    TxHistory    string `json:"tx_history,omitempty"`
}
```

## 🌐 HTTP/JSON Request Format

Since this is a gRPC-Gateway endpoint, you make HTTP requests with JSON:

### Minimal Required Request

```bash
curl -X POST "http://localhost:8080/v1/voice/note/process" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -d '{
    "audio_content": "BASE64_ENCODED_AUDIO_DATA"
  }'
```

### Complete Request with All Fields

```bash
curl -X POST "http://localhost:8080/v1/voice/note/process" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -d '{
    "audio_content": "UklGRiQAAABXQVZFZm10IBAAAAABAAEARKwAAIhYAQACABAAZGF0YQAAAAA=",
    "filename": "voice_message.wav",
    "content_type": "audio/wav",
    "tx_history": "[{\"type\":\"transfer\",\"amount\":100.50,\"currency\":\"USD\",\"status\":\"COMPLETED\",\"created_at\":\"2024-01-15T10:30:00Z\",\"description\":\"Payment to John\"}]"
  }'
```

## 📝 Field Details

### 1. audio_content (Required)

- **Type**: `bytes` (base64-encoded in JSON)
- **Description**: Binary audio content
- **Max Size**: 25MB
- **Supported Formats**: `.mp3`, `.wav`, `.m4a`, `.ogg`, `.flac`, `.aac`, `.mp4`, `.webm`

**Example**:
```bash
# Convert audio file to base64
BASE64_AUDIO=$(base64 -i voice_message.wav)
echo "\"audio_content\": \"$BASE64_AUDIO\""
```

### 2. filename (Optional)

- **Type**: `string`
- **Description**: Original filename of the audio file
- **Default**: `"voice_note.wav"` if not provided

**Example**:
```json
"filename": "my_voice_note.wav"
```

### 3. content_type (Optional)

- **Type**: `string`
- **Description**: MIME type of the audio file
- **Common Values**: 
  - `"audio/wav"`
  - `"audio/mp3"`
  - `"audio/mpeg"`
  - `"audio/m4a"`
  - `"audio/ogg"`

**Example**:
```json
"content_type": "audio/wav"
```

### 4. tx_history (Optional)

- **Type**: `string` (JSON string)
- **Description**: Recent transaction history as JSON string
- **Default**: Service will fetch recent transactions if not provided

**Example**:
```json
"tx_history": "[{\"type\":\"transfer\",\"amount\":100.50,\"currency\":\"USD\",\"status\":\"COMPLETED\",\"created_at\":\"2024-01-15T10:30:00Z\",\"description\":\"Payment to John\"}]"
```

## 🔐 Authentication

**Required**: JWT Bearer token in Authorization header

```bash
-H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
```

The JWT token must contain valid user information. The service extracts user ID and other details from the token.

## 📤 Response Format

### ProcessVoiceNoteResponse

```protobuf
message ProcessVoiceNoteResponse {
    bool success = 1;               // Processing success status
    string msg = 2;                 // Status message
    string response = 3;            // AI response text
    string transcribed_text = 4;    // Transcribed audio text
    int64 processing_time_ms = 5;   // Processing time in milliseconds
}
```

### Success Response Example

```json
{
  "success": true,
  "msg": "Voice note processed successfully",
  "response": "I heard you mention a payment. Based on your recent transactions, you've sent $100.50 to John. Would you like me to help you with anything else regarding your payments?",
  "transcribed_text": "Hey, can you help me check my recent payment to John?",
  "processing_time_ms": 2340
}
```

### Error Response Example

```json
{
  "success": false,
  "msg": "Audio file size exceeds 25MB limit",
  "response": "",
  "transcribed_text": "",
  "processing_time_ms": 0
}
```

## 🧪 Complete Test Examples

### 1. Simple Test with Audio File

```bash
# Step 1: Create base64 audio
BASE64_AUDIO=$(base64 -i your_audio.wav)

# Step 2: Make request
curl -X POST "http://localhost:8080/v1/voice/note/process" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -d "{
    \"audio_content\": \"$BASE64_AUDIO\",
    \"filename\": \"your_audio.wav\",
    \"content_type\": \"audio/wav\"
  }"
```

### 2. Test with Custom Transaction History

```bash
BASE64_AUDIO=$(base64 -i your_audio.wav)

curl -X POST "http://localhost:8080/v1/voice/note/process" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -d "{
    \"audio_content\": \"$BASE64_AUDIO\",
    \"filename\": \"your_audio.wav\",
    \"content_type\": \"audio/wav\",
    \"tx_history\": \"[{\\\"type\\\":\\\"transfer\\\",\\\"amount\\\":250.75,\\\"currency\\\":\\\"USD\\\",\\\"status\\\":\\\"COMPLETED\\\",\\\"created_at\\\":\\\"2024-01-15T14:30:00Z\\\",\\\"description\\\":\\\"Coffee payment\\\"}]\"
  }"
```

### 3. Minimal Request (Only Required Fields)

```bash
BASE64_AUDIO=$(base64 -i your_audio.wav)

curl -X POST "http://localhost:8080/v1/voice/note/process" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -d "{
    \"audio_content\": \"$BASE64_AUDIO\"
  }"
```

## ⚡ Quick Test Script

```bash
#!/bin/bash

# Replace with your values
JWT_TOKEN="YOUR_JWT_TOKEN_HERE"
AUDIO_FILE="your_audio.wav"
SERVER_URL="http://localhost:8080"

# Convert audio to base64
BASE64_AUDIO=$(base64 -i "$AUDIO_FILE")

# Make the request
curl -X POST "$SERVER_URL/v1/voice/note/process" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $JWT_TOKEN" \
  -d "{
    \"audio_content\": \"$BASE64_AUDIO\",
    \"filename\": \"$AUDIO_FILE\",
    \"content_type\": \"audio/wav\"
  }" | jq .
```

## 🔧 Environment Setup

Make sure these are configured:

```bash
export AI_SERVICE_URL=http://localhost:8000    # Django AI service URL
export JWT_SECRET_KEY=your_jwt_secret_here     # JWT signing secret
export PORT=8080                               # Server port
```

## 📊 Validation Rules

1. **audio_content**: Must be valid base64-encoded audio data
2. **File size**: Maximum 25MB after decoding
3. **File format**: Must have valid audio extension if filename provided
4. **Authentication**: Valid JWT token required
5. **tx_history**: Must be valid JSON string if provided 