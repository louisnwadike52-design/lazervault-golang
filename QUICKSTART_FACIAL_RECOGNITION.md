# 🚀 Quick Start: Facial Recognition Service

This guide will get you up and running with the facial recognition gRPC-REST gateway in 5 minutes.

## ✅ Prerequisites

- Go 1.21+
- Docker & Docker Compose
- A running Django facial recognition service (or use the mock service below)

## 🏃‍♂️ Quick Setup

### 1. Build the Project
```bash
# Generate protobuf files
make proto

# Build the application
go build -v
```

### 2. Configure Environment
Update your `app.env` file:
```bash
AI_SERVICE_URL=http://localhost:8000  # Your AI service URL (includes facial recognition)
HTTP_SERVER_PORT=8080
GRPC_SERVER_PORT=9090
```

### 3. Start the Service
```bash
# Option A: Run locally (requires Django service running)
go run main.go

# Option B: Use Docker Compose with all services
make facial-recognition-run
```

## 🧪 Test the Service

### Test Health Check
```bash
curl -X GET "http://localhost:8080/v1/face/health"
```

**Expected Response:**
```json
{
  "healthy": true,
  "message": "Service is healthy",
  "service_version": "1.0.0",
  "timestamp": 1703123456
}
```

### Test Face Registration
```bash
# Create a test image (or use your own)
echo "dummy test image" > test_face.jpg

# Register a face
curl -X POST "http://localhost:8080/v1/face/register" \
  -F "user_id=test_user_123" \
  -F "face_id=primary" \
  -F "allow_duplicates=false" \
  -F "duplicate_threshold=0.85" \
  -F "image=@test_face.jpg"
```

### Test Face Verification
```bash
curl -X POST "http://localhost:8080/v1/face/verify" \
  -F "user_id=test_user_123" \
  -F "threshold=0.7" \
  -F "image=@test_face.jpg"
```

### Run Automated Tests
```bash
# Run the comprehensive test suite
make facial-recognition-test

# Or run manually
./scripts/test-facial-recognition.sh
```

## 🔧 Mock Django Service (For Testing)

If you don't have a Django facial recognition service yet, here's a simple mock server:

```python
# mock_facial_service.py
from flask import Flask, request, jsonify
import json

app = Flask(__name__)

@app.route('/api/health', methods=['GET'])
def health():
    return jsonify({"status": "healthy"})

@app.route('/api/face/register', methods=['POST'])
def register_face():
    user_id = request.form.get('user_id')
    face_id = request.form.get('face_id', 'default')
    
    return jsonify({
        "success": True,
        "face_id": face_id,
        "message": "Face registered successfully (mock)",
        "num_faces_detected": 1,
        "duplicate_details": {
            "is_duplicate": False,
            "threshold": 0.85,
            "total_matches": 0,
            "message": "No duplicates found"
        }
    })

@app.route('/api/face/verify', methods=['POST'])
def verify_face():
    user_id = request.form.get('user_id')
    
    return jsonify({
        "success": True,
        "verified": True,
        "confidence": 0.92,
        "matched_face_id": "primary",
        "threshold": 0.7,
        "distance": 0.08,
        "message": "Face verified successfully (mock)"
    })

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=8000, debug=True)
```

Run the mock service:
```bash
# Install Flask if not installed
pip install flask

# Run the mock service
python mock_facial_service.py
```

## 📊 Service Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   REST Client   │────│  Go Gateway     │────│ Django Service  │
│                 │    │                 │    │                 │
│ • cURL          │    │ • gRPC Server   │    │ • Face Register │
│ • Frontend      │    │ • REST Gateway  │    │ • Face Verify   │
│ • Mobile App    │    │ • Validation    │    │ • Health Check  │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

## 🎯 Key Features Implemented

✅ **REST API Endpoints**
- `POST /v1/face/register` - Register faces with duplicate detection
- `POST /v1/face/verify` - Verify faces against registered ones  
- `GET /v1/face/health` - Service health monitoring

✅ **gRPC Service**
- Protocol buffer definitions
- Generated client/server code
- Gateway integration

✅ **Production Features**
- Circuit breaker pattern
- Retry logic with exponential backoff
- Request validation
- Error handling and mapping
- Security headers
- File upload support

✅ **Monitoring & Observability**
- Health checks
- Structured logging
- Docker containerization
- Test automation

## 🚀 Next Steps

1. **Deploy to Production**
   ```bash
   # Build Docker image
   make facial-recognition-build
   
   # Deploy with Docker Compose
   docker-compose -f docker-compose.facial-recognition.yml up -d
   ```

2. **Add Authentication**
   - Configure JWT tokens in environment
   - Add API key validation
   - Set up rate limiting

3. **Enable Monitoring**
   - Configure Prometheus metrics
   - Set up Grafana dashboards
   - Add alerting rules

4. **Scale the Service**
   - Deploy multiple instances
   - Add load balancing
   - Configure auto-scaling

## 🆘 Troubleshooting

**Service won't start?**
```bash
# Check configuration
cat app.env

# Verify AI service is running
curl http://localhost:8000/api/health

# Check logs
docker-compose logs facial-recognition-gateway
```

**Tests failing?**
```bash
# Verify service is running
curl http://localhost:8080/v1/face/health

# Run individual test components
./scripts/test-facial-recognition.sh
```

**Build errors?**
```bash
# Regenerate proto files
make proto

# Clean and rebuild
go clean -cache
go build -v
```

## 📞 Support

- Check the full documentation: `FACIAL_RECOGNITION_INTEGRATION.md`
- Review the API specifications in `swagger/api.swagger.json`
- Run the test suite for validation: `make facial-recognition-test`

---

**🎉 Congratulations!** You now have a production-ready facial recognition gateway service running. The service automatically handles multipart file uploads, proxies requests to your Django backend, and provides comprehensive error handling and monitoring capabilities. 