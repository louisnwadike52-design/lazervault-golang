# Flutter Voice Note Integration Prompt

## Prompt for Cursor Chat in Flutter App

```
I need to implement voice note functionality in my Flutter app that communicates with my LazerVault Golang backend. Here are the complete specifications:

## BACKEND API DETAILS

**Endpoint**: POST /v1/voice/note/upload
**Content-Type**: multipart/form-data
**Base URL**: [YOUR_API_BASE_URL] (e.g., https://api.lazervault.com or http://localhost:8080)

### Request Format:
- Method: POST
- Content-Type: multipart/form-data
- Fields:
  - audio_file: File (required) - Audio file (.mp3, .wav, .m4a, .ogg, .flac, .aac, .mp4, .webm, max 25MB)
  - user_id: string (required) - Current user's ID
  - tx_history: string (optional) - JSON string of recent transactions
  - access_token: string (optional) - User's auth token

### Response Format:
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
- 400 Bad Request: Invalid file format, size too large, missing required fields
- 401 Unauthorized: Invalid user_id or access_token
- 413 Payload Too Large: File exceeds 25MB
- 500 Internal Server Error: Backend processing error
- 503 Service Unavailable: AI service unavailable

## IMPLEMENTATION REQUIREMENTS

I need you to help me implement:

1. **Audio Recording Widget**:
   - Record button with visual feedback (recording/stopped states)
   - Real-time recording duration display
   - Stop/cancel recording functionality
   - Audio playback preview before sending
   - Support for common audio formats (prefer .wav or .m4a)

2. **File Upload Service**:
   - HTTP multipart form data upload
   - Progress indicator during upload
   - Proper error handling with user-friendly messages
   - Timeout handling (60 seconds for voice processing)
   - Retry mechanism for failed uploads

3. **UI Components**:
   - Voice note input widget (similar to WhatsApp voice messages)
   - Loading states during transcription/AI processing
   - Display transcribed text and AI response
   - Error states with retry options
   - Integration with existing chat/messaging UI

4. **State Management**:
   - Recording state (idle, recording, processing, completed, error)
   - Upload progress tracking
   - Response data handling
   - Error state management

## TECHNICAL SPECIFICATIONS

### Dependencies Needed:
```yaml
dependencies:
  http: ^1.1.0
  path_provider: ^2.1.1
  permission_handler: ^11.0.1
  # For audio recording - choose ONE:
  record: ^5.0.4  # Recommended
  # OR
  flutter_sound: ^9.2.13
  # OR  
  audio_waveforms: ^1.0.5
```

### Audio Requirements:
- Record in .wav or .m4a format
- Sample rate: 44.1kHz or 22.05kHz
- Bit depth: 16-bit minimum
- Mono or stereo acceptable
- Maximum duration: 5 minutes (to stay under 25MB limit)
- Minimum duration: 1 second

### User Flow:
1. User taps record button
2. Request microphone permission if needed
3. Start recording with visual feedback
4. User can stop recording or cancel
5. Show preview with play/re-record/send options
6. On send: Upload to backend with loading indicator
7. Display transcription and AI response
8. Handle errors gracefully with retry options

## INTEGRATION POINTS

### Authentication:
- Get user_id from your existing auth state/provider
- Get access_token from your existing auth system
- Handle auth errors (token refresh if needed)

### Transaction History:
- If you have recent transactions available, format as JSON string:
```dart
final txHistory = jsonEncode([
  {
    "id": "tx1",
    "type": "transfer", 
    "amount": 250,
    "currency": "USD",
    "status": "completed",
    "created_at": "2024-01-15T10:30:00Z"
  }
  // ... up to 5 most recent transactions
]);
```

### Error Handling:
- Network connectivity issues
- Microphone permission denied
- File size exceeded
- Unsupported audio format
- Backend service errors
- Timeout scenarios

## CODE STRUCTURE REQUESTS

Please create:

1. **VoiceNoteService** class for API communication
2. **AudioRecorderWidget** for recording UI
3. **VoiceNoteResponse** model class
4. **VoiceNoteUploadState** for state management
5. **Error handling utilities** for different error types
6. **Permission handling** for microphone access

## EXAMPLE USAGE

I want to be able to use it like this:
```dart
VoiceNoteWidget(
  onVoiceNoteComplete: (VoiceNoteResponse response) {
    // Handle the AI response and transcription
    print('Transcribed: ${response.transcribedText}');
    print('AI Response: ${response.response}');
  },
  onError: (String error) {
    // Handle errors
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(content: Text(error))
    );
  },
)
```

## ADDITIONAL CONSIDERATIONS

- **Performance**: Handle large audio files efficiently
- **User Experience**: Smooth animations and feedback
- **Accessibility**: Voice commands, screen reader support
- **Platform Differences**: iOS vs Android microphone handling
- **Background Processing**: Handle app backgrounding during recording
- **File Cleanup**: Remove temporary audio files after upload

Please provide a complete, production-ready implementation with proper error handling, state management, and user experience considerations. Include comments explaining the key parts and any platform-specific considerations.
```

## Additional Context for Flutter Developer

Use this prompt in your Cursor chat to get a complete voice note implementation. The prompt includes:

✅ **Complete API Specifications** - Exact endpoint details and request/response formats
✅ **Technical Requirements** - Audio formats, file sizes, dependencies
✅ **User Experience Flow** - Step-by-step interaction design
✅ **Error Handling** - Comprehensive error scenarios and responses
✅ **Integration Points** - How to connect with existing auth and transaction systems
✅ **Code Structure** - Requested classes and architecture
✅ **Production Considerations** - Performance, accessibility, platform differences

The prompt is designed to give Cursor's AI all the context needed to generate a robust, production-ready voice note feature that integrates seamlessly with your LazerVault backend. 