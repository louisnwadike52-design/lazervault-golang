.PHONY: proto create-dirs tail tail-http tail-http2 deploy-http deploy-http2

PROJECT_ID := lazervault
CLOUDRUN_REGION := europe-west1
CLOUDRUN_SERVICE_NAME_HTTP := lazervault-golang-http
CLOUDRUN_SERVICE_NAME_HTTP2 := lazervault-golang-http2

deploy-http:
	@echo "Submitting build to Google Cloud Build for project lazervault..."
	gcloud builds submit --region=europe-west1 --project=lazervault --config=cloudbuild.http.yaml

deploy-http2:
	@echo "Submitting build to Google Cloud Build for project lazervault..."
	gcloud builds submit --region=europe-west1 --project=lazervault --config=cloudbuild.http2.yaml

deploy-go-microservices:
	@echo "Submitting build to Google Cloud Build for project lazervault..."
	gcloud builds submit --region=europe-west1 --project=lazervault --config=cloudbuild.go-microservices.yaml

create-dirs:
	mkdir -p pb
	mkdir -p swagger

proto: create-dirs
	@echo "Generating protobuf files..."
	protoc \
		--proto_path=proto \
		--proto_path=../../microservices/accounts-service/accounts-microservice/proto \
		--proto_path=proto/google/api \
		--proto_path=proto/protoc-gen-openapiv2/options \
		--go_out=./pb --go_opt=paths=source_relative \
		--go-grpc_out=./pb --go-grpc_opt=paths=source_relative \
		--grpc-gateway_out=./pb --grpc-gateway_opt=paths=source_relative \
		--grpc-gateway_opt=allow_repeated_fields_in_body=true \
		--openapiv2_out=swagger \
		--openapiv2_opt=allow_merge=true,merge_file_name=api \
		proto/*.proto

tail-http:
	@echo "Tailing logs for Cloud Run service: $(CLOUDRUN_SERVICE_NAME_HTTP)..."
	@gcloud run services logs tail $(CLOUDRUN_SERVICE_NAME_HTTP) --project=$(PROJECT_ID) --region=$(CLOUDRUN_REGION) -f Dockerfile.http

tail-http2:
	@echo "Tailing logs for Cloud Run service: $(CLOUDRUN_SERVICE_NAME_HTTP2)..."
	@gcloud run services logs tail $(CLOUDRUN_SERVICE_NAME_HTTP2) --project=$(PROJECT_ID) --region=$(CLOUDRUN_REGION) -f Dockerfile.http2

grant-ADC: 
	gcloud auth application-default set-quota-project lazervault-460507

# Facial Recognition Service Commands
facial-recognition-build:
	@echo "Building facial recognition gateway..."
	docker build -f Dockerfile.facial-recognition -t facial-recognition-gateway:latest .

facial-recognition-run:
	@echo "Running facial recognition services..."
	docker-compose -f docker-compose.facial-recognition.yml up --build

facial-recognition-test:
	@echo "Testing facial recognition endpoints..."
	./scripts/test-facial-recognition.sh

facial-recognition-clean:
	@echo "Cleaning up facial recognition containers..."
	docker-compose -f docker-compose.facial-recognition.yml down -v

