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


	{
    "account": {
        "id": "3",
        "account_type": "Private",
        "currency": "USD",
        "balance": 0,
        "status": "active",
        "card_holder_name": "Louis Nwadike",
        "card_type": "debit",
        "expiry_date": "04/28",
        "daily_limit": 5000,
        "monthly_limit": 50000,
        "enable_3d_secure": true,
        "enable_contactless": true,
        "enable_online_payments": true,
        "account_number": "LV174569909492925300014",
        "iban": "",
        "bic_swift": "",
        "created_at": {
            "seconds": "1745699094",
            "nanos": 929431000
        },
        "updated_at": {
            "seconds": "1745699094",
            "nanos": 929431000
        }
    }
}