# Flutter Voice Note gRPC Integration Prompt

## Prompt for Cursor Chat in Flutter App (gRPC Endpoint)

```
I need to implement voice note functionality in my Flutter app that communicates with my LazerVault Golang backend via gRPC, similar to how my existing AI chat (processchat) works. Here are the complete specifications:

## BACKEND API DETAILS

**Endpoint**: POST /v1/voice/note/process
**Content-Type**: application/json
**Base URL**: [YOUR_API_BASE_URL] (e.g., https://api.lazervault.com or http://localhost:8080)

### gRPC Request Format (via HTTP Gateway):
```json
{
  "audio_content": "base64_encoded_audio_bytes",
  "filename": "voice_note.wav",
  "content_type": "audio/wav",
  "tx_history": "[{\"id\":\"tx1\",\"amount\":250}]"  // Optional JSON string
}
```

### Request Headers:
```
Authorization: Bearer YOUR_JWT_TOKEN
Content-Type: application/json
```

### Response Format (same as processchat pattern):
```json
{
  "success": true|false,
  "msg": "Status message",
  "response": "AI response text based on voice note",
  "transcribed_text": "Transcribed text from the audio",
  "processing_time_ms": 1247
}
```

### Error Responses:
- 400 Bad Request: Invalid audio format, size too large, missing audio_content
- 401 Unauthorized: Invalid or missing JWT token
- 413 Payload Too Large: Audio exceeds 25MB when base64 encoded
- 500 Internal Server Error: Backend processing error
- 503 Service Unavailable: AI microservice unavailable

## IMPLEMENTATION REQUIREMENTS

I need you to help me implement this EXACTLY like my existing processchat functionality, but for voice notes:

1. **Audio Recording Widget**:
   - Record button with visual feedback (recording/stopped states)
   - Real-time recording duration display
   - Stop/cancel recording functionality
   - Audio playback preview before sending
   - Convert audio file to base64 for gRPC transmission

2. **gRPC Service (HTTP Gateway)**:
   - Use same HTTP client pattern as processchat
   - Send audio as base64 encoded binary in JSON body
   - Include JWT token in Authorization header
   - Handle same timeout (60 seconds for voice processing)
   - Follow same error handling pattern as processchat

3. **UI Components**:
   - Voice note input widget (similar to WhatsApp voice messages)
   - Loading states during transcription/AI processing
   - Display transcribed text and AI response
   - Error states with retry options
   - Integration with existing chat/messaging UI

4. **State Management**:
   - Use same state management pattern as processchat
   - Recording state (idle, recording, processing, completed, error)
   - Response data handling
   - Error state management

## TECHNICAL SPECIFICATIONS

### Dependencies Needed:
```yaml
dependencies:
  http: ^1.1.0
  path_provider: ^2.1.1
  permission_handler: ^11.0.1
  # For audio recording:
  record: ^5.0.4  # Recommended
  # For base64 encoding:
  dart:convert  # Built-in
```

### Audio Requirements:
- Record in .wav, .m4a, or .mp3 format
- Convert to base64 for transmission
- Maximum file size: ~18MB (to stay under 25MB after base64 encoding)
- Sample rate: 44.1kHz or 22.05kHz
- Bit depth: 16-bit minimum
- Maximum duration: 3-4 minutes (to stay under size limit)

### Authentication (Same as ProcessChat):
- Use existing JWT token from your auth provider
- Include in Authorization header: "Bearer {token}"
- Handle 401 errors same as processchat (token refresh)

### User Flow:
1. User taps record button
2. Request microphone permission if needed
3. Start recording with visual feedback
4. User stops recording
5. Convert audio file to base64
6. Send to gRPC endpoint with loading indicator
7. Display transcription and AI response
8. Handle errors same as processchat

## INTEGRATION POINTS (Follow ProcessChat Pattern)

### Service Layer:
Create `VoiceNoteService` that mirrors your existing AI chat service:
```dart
class VoiceNoteService {
  static const String endpoint = '/v1/voice/note/process';
  
  Future<VoiceNoteResponse> processVoiceNote({
    required Uint8List audioBytes,
    required String filename,
    required String contentType,
    String? txHistory,
  }) async {
    // Similar to your processchat implementation
    // Convert audioBytes to base64
    // Send HTTP POST request
    // Handle response/errors
  }
}
```

### Request Body Structure:
```dart
final requestBody = {
  'audio_content': base64Encode(audioBytes),
  'filename': filename,
  'content_type': contentType,
  'tx_history': txHistory ?? await _getRecentTransactions(),
};
```

### Response Model:
```dart
class VoiceNoteResponse {
  final bool success;
  final String msg;
  final String response;
  final String transcribedText;
  final int processingTimeMs;
  
  factory VoiceNoteResponse.fromJson(Map<String, dynamic> json) {
    // Parse same as processchat response
  }
}
```

### Error Handling (Same as ProcessChat):
- Network connectivity issues
- JWT token expired/invalid
- File size exceeded
- Backend service errors
- Timeout scenarios
- Use same error handling utilities as processchat

## CODE STRUCTURE REQUESTS

Please create these components following the EXACT same patterns as my processchat implementation:

1. **VoiceNoteService** - API service (mirror processchat service)
2. **VoiceNoteResponse** - Response model (similar to processchat response)
3. **AudioRecorderWidget** - Recording UI component
4. **VoiceNoteState** - State management (follow processchat state pattern)
5. **Error handling** - Use same error handling as processchat
6. **Base64 conversion utilities** - For audio file encoding

## EXAMPLE USAGE (Same Pattern as ProcessChat)

```dart
VoiceNoteWidget(
  onVoiceNoteComplete: (VoiceNoteResponse response) {
    // Handle same as processchat response
    if (response.success) {
      print('Transcribed: ${response.transcribedText}');
      print('AI Response: ${response.response}');
      // Add to chat history, etc.
    }
  },
  onError: (String error) {
    // Handle errors same as processchat
    showErrorSnackBar(error);
  },
)
```

## KEY DIFFERENCES FROM MULTIPART UPLOAD

1. **No multipart form data** - Send as JSON with base64 audio
2. **Use gRPC HTTP Gateway** - Same endpoint pattern as processchat
3. **JWT Authentication** - Same as processchat (Bearer token)
4. **Base64 Encoding** - Convert audio bytes to base64 string
5. **Same Error Handling** - Mirror processchat error patterns
6. **Same Service Architecture** - Follow processchat service structure

## AUDIO TO BASE64 CONVERSION

```dart
// Convert recorded audio file to base64
Future<String> audioFileToBase64(String filePath) async {
  final file = File(filePath);
  final bytes = await file.readAsBytes();
  return base64Encode(bytes);
}
```

## TRANSACTION HISTORY INTEGRATION

If you have recent transactions available (same as processchat):
```dart
Future<String> _getRecentTransactions() async {
  // Get from your existing transaction service
  final transactions = await transactionService.getRecentTransactions(limit: 5);
  return jsonEncode(transactions.map((tx) => tx.toJson()).toList());
}
```

## REQUEST EXAMPLE

Final HTTP request should look like:
```dart
POST /v1/voice/note/process
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
Content-Type: application/json

{
  "audio_content": "UklGRigAAABXQVZFZm10IBAAAAABAAEAQB8AAEAfAAABAAgAZGF0YQQAAAAB...",
  "filename": "voice_note_20240115_103000.wav",
  "content_type": "audio/wav",
  "tx_history": "[{\"id\":\"tx1\",\"type\":\"transfer\",\"amount\":250}]"
}
```

Please provide a complete implementation that follows the EXACT same patterns as my processchat functionality, just adapted for voice notes. Include proper base64 encoding, same authentication flow, same error handling, and same service architecture.
```

## Key Changes for gRPC Approach

This prompt specifically targets:

✅ **gRPC HTTP Gateway** - Uses `/v1/voice/note/process` endpoint
✅ **Base64 Encoding** - Converts audio to base64 for JSON transmission  
✅ **JWT Authentication** - Same Bearer token pattern as processchat
✅ **Same Service Architecture** - Mirrors your existing AI chat implementation
✅ **JSON Request Body** - No multipart, pure JSON like processchat
✅ **Error Handling** - Follows processchat error patterns
✅ **State Management** - Uses same patterns as your existing chat

The key difference is that the Flutter app will:
1. Record audio file
2. Convert to base64 
3. Send via gRPC HTTP gateway (like processchat)
4. Your Golang service forwards to AI microservice
5. Response follows same pattern as processchat 