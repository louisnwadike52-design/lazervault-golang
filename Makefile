# Variables
BINARY_NAME=lazervaultGo
DB_NAME=lazervault_db

# Go related variables
GOBASE=$(shell pwd)
GOBIN=$(GOBASE)/bin

# Main commands
.PHONY: all build clean run watch test

all: clean build

build:
	@echo "Go application is now built inside the Docker image. See Dockerfile."
	@echo "To build locally (e.g., for testing outside Docker), you can run:"
	@echo "  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags='"-w -s"' -o $(GOBIN)/$(BINARY_NAME) main.go"

clean:
	@echo "Cleaning..."
	@rm -rf $(GOBIN)
	@go clean
	rm -rf pb/*.go
	rm -rf swagger/*.json

run:
	@go run main.go

# Watch mode using air
watch:
	@echo "Starting in watch mode..."
	@air

# Database commands
.PHONY: db-create db-drop db-reset

db-create:
	@echo "Creating database..."
	@createdb -U postgres $(DB_NAME)

db-drop:
	@echo "Dropping database..."
	@dropdb -U postgres $(DB_NAME) --if-exists

db-reset: db-drop db-create

# Development setup
.PHONY: setup install-air

setup:
	@echo "Setting up development environment..."
	@./scripts/setup.sh

install-air:
	@echo "Installing air..."
	@go install github.com/air-verse/air@latest

migrate:
	@echo "Migrating..."
	@go run main.go -migrate

# Help
.PHONY: help
help:
	@echo "Available commands:"
	@echo "  make build      - Build the application"
	@echo "  make clean      - Clean build files"
	@echo "  make run        - Run the application"
	@echo "  make watch      - Run in watch mode using air"
	@echo "  make db-create  - Create database"
	@echo "  make db-drop    - Drop database"
	@echo "  make db-reset   - Reset database"
	@echo "  make setup      - Setup development environment"

.PHONY: proto proto-deps install-tools

# Install all required protoc plugins #install these manually one by one
install-tools:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest
	go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@latest

# Install redis
install-redis:
	brew install redis

# Download proto dependencies
proto-deps:
	./scripts/proto-deps.sh

# Generate proto files
proto: create-dirs
	protoc \
		--proto_path=proto \
		--proto_path=proto/google/api \
		--proto_path=proto/protoc-gen-openapiv2/options \
		--go_out=./pb --go_opt=paths=source_relative \
		--go-grpc_out=./pb --go-grpc_opt=paths=source_relative \
		--grpc-gateway_out=./pb --grpc-gateway_opt=paths=source_relative \
		--grpc-gateway_opt=allow_repeated_fields_in_body=true \
		--openapiv2_out=swagger \
		--openapiv2_opt=allow_merge=true,merge_file_name=api \
		proto/*.proto

.PHONY: evans
evans:
	evans --host localhost --port 50051 -r repl

.PHONY: dev
dev:
	nodemon

.PHONY: create-dirs
create-dirs:
	mkdir -p pb
	mkdir -p swagger

.PHONY: install-protoc-plugins
install-protoc-plugins:
	@echo "Installing protoc plugins..."
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest
	go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@latest
	@echo "Protoc plugins installed."

# --- Artifact Registry Configuration --- #
# !!! Values should have NO leading/trailing spaces themselves !!!
AR_LOCATION_VAL      := europe-west1
PROJECT_ID_VAL       := lazervault
AR_REPOSITORY_VAL    := lazervault-images
IMAGE_BASENAME_VAL   := lazervault-golang

# Strip them once to be sure
AR_LOCATION      := $(strip $(AR_LOCATION_VAL))
PROJECT_ID       := $(strip $(PROJECT_ID_VAL))
AR_REPOSITORY    := $(strip $(AR_REPOSITORY_VAL))
IMAGE_BASENAME   := $(strip $(IMAGE_BASENAME_VAL))

# Construct the full image name carefully
FULL_IMG_NAME    := $(AR_LOCATION)-docker.pkg.dev/$(PROJECT_ID)/$(AR_REPOSITORY)/$(IMAGE_BASENAME)
TAG              := latest # Or use $(shell git rev-parse --short HEAD)

# --- GKE Authentication --- #
GKE_CLUSTER_NAME_VAL := lazervault
GKE_ZONE_VAL         := europe-west1 # As specified by user

GKE_CLUSTER_NAME   := $(strip $(GKE_CLUSTER_NAME_VAL))
GKE_ZONE           := $(strip $(GKE_ZONE_VAL))

.PHONY: gke-auth
gke-auth:
	@echo "Ensuring gke-gcloud-auth-plugin is installed..."
	@gcloud components install gke-gcloud-auth-plugin --quiet
	@echo "Authenticating to GKE cluster: $(GKE_CLUSTER_NAME) in zone $(GKE_ZONE)..."
	@gcloud container clusters get-credentials $(GKE_CLUSTER_NAME) --zone $(GKE_ZONE) --project $(PROJECT_ID)
	@echo "Successfully configured kubectl for GKE cluster."

# --- GKE Deployment Targets --- #

# Main GKE deployment command
deploy: gke-auth build-gke push-gke apply-k8s-manifests
	@echo "GKE deployment process initiated successfully."
	@echo "Run 'make status-gke' to check deployment status."
	@echo "Run 'make logs-gke' to view application logs."

build-gke:
	@echo "Building GKE Docker image: $(FULL_IMG_NAME):$(TAG)"
	docker build -t $(FULL_IMG_NAME):$(TAG) -f Dockerfile .

push-gke:
	@echo "Pushing GKE Docker image to Artifact Registry: $(FULL_IMG_NAME):$(TAG)"
	docker push $(FULL_IMG_NAME):$(TAG)

apply-k8s-manifests:
	@echo "Applying Kubernetes manifests to GKE..."
	kubectl apply -f k8s/configmap.yaml       # Apply ConfigMap (if kept, otherwise remove)
	kubectl apply -f k8s/redis-deployment.yaml
	kubectl apply -f k8s/redis-service.yaml
	# Potentially kubectl apply -f k8s/secrets.yaml (if you manage K8s secrets separately)
	# Update deployment.yaml image path before applying if not using variables directly
	@echo "Ensuring deployment.yaml uses image: $(FULL_IMG_NAME):$(TAG)"
	# This sed command is an attempt to update the image path in deployment.yaml. 
	# CAUTION: This is a simple replacement and might fail if image path format changes drastically.
	# It's safer to ensure your deployment.yaml is already correct or uses a templating tool.
	sed -i.bak 's|image: .*|image: $(FULL_IMG_NAME):$(TAG)|g' k8s/deployment.yaml && rm k8s/deployment.yaml.bak
	kubectl apply -f k8s/deployment.yaml
	kubectl apply -f k8s/grpc-service.yaml
	kubectl apply -f k8s/rest-service.yaml
	@echo "Kubernetes manifests applied."
	@echo "Deployment initiated. Check status with: kubectl get all -l app=lazervault ; kubectl get all -l app=redis"
	@echo "To get LoadBalancer IPs: kubectl get svc lazervault-grpc-service lazervault-rest-service"

# Helper targets for GKE
status-gke:
	@echo "Application Pods (lazervault):"
	kubectl get pods -l app=lazervault
	@echo "\nRedis Pods:"
	kubectl get pods -l app=redis
	@echo "\nApplication Services (lazervault):"
	kubectl get svc -l app=lazervault
	@echo "\nRedis Service:"
	kubectl get svc redis-service

logs-gke:
	@echo "Tailing logs for lazervault-app containers..."
	kubectl logs -l app=lazervault -c lazervault-app --follow

get-gke-ip:
	@echo "Getting GKE IP addresses..."
	kubectl get svc lazervault-grpc-service lazervault-rest-service

# Combined target (kept for compatibility if previously used, but 'deploy' is now primary)
release-gke: deploy 

# To delete all GKE resources defined in k8s/
delete-gke:
	@echo "Deleting GKE resources..."
	kubectl delete pods -l app=lazervault
	kubectl delete -f k8s/rest-service.yaml --ignore-not-found=true
	kubectl delete -f k8s/grpc-service.yaml --ignore-not-found=true
	kubectl delete -f k8s/deployment.yaml --ignore-not-found=true
	kubectl delete -f k8s/redis-service.yaml --ignore-not-found=true
	kubectl delete -f k8s/redis-deployment.yaml --ignore-not-found=true
	kubectl delete -f k8s/configmap.yaml --ignore-not-found=true
	# kubectl delete secret lazervault-secrets --ignore-not-found=true # If you create it
	@echo "GKE resources deleted."

.PHONY: force-restart
force-restart:
	@echo "Forcing pod restart by updating deployment annotation..."
	kubectl patch deployment lazervault-deployment \
		--patch "{\"spec\":{\"template\":{\"metadata\":{\"annotations\":{\"kubectl.kubernetes.io/restartedAt\":\"$(shell date -u +%Y-%m-%dT%H:%M:%SZ)\"}}}}}"

delete: delete-gke
	@echo "Deleting GKE cluster..."
	@gcloud container clusters delete lazervault --region=europe-west1 --project=lazervault --quiet
	@echo "GKE cluster deleted."

create-cluster:
	@echo "Creating GKE cluster..."
	@gcloud container clusters create lazervault --machine-type=e2-standard-4 --num-nodes=3 --enable-autoscaling --min-nodes=1 --max-nodes=5 --enable-ip-alias --release-channel=regular --enable-autoupgrade --enable-autorepair --region=europe-west1 --project=lazervault --quiet
	@echo "GKE cluster created."
		

# The old App Engine deploy target - keep it if you still need it, otherwise remove.
# Ensure it doesn't conflict with the new 'deploy' for GKE.
# deploy-appengine:
# 	@echo "Deploying to App Engine Flex..."
# 	@gcloud app deploy app.yaml --project=lazervault --quiet

# grant-permissions for App Engine (if needed)
# grant-permissions-appengine:
# 	 gcloud ...
