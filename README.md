# lazervault-golang

#install dependencies
install-tools:
	go install \
		github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest \
		github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@latest \
		google.golang.org/protobuf/cmd/protoc-gen-go@latest \
		google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest \
        github.com/grpc-ecosystem/grpc-gateway/v2/runtime \

#Adding grpc-http gateway dependencies: 
go get \
    github.com/grpc-ecosystem/grpc-gateway/v2/runtime \
    github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway \
    github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2