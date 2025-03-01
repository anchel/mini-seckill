NAME=seckill
BUILD_DIR=build

pb:
	protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative proto/seckill.proto

run:
	go build -o ${BUILD_DIR}/seckill main.go
	cp .env ${BUILD_DIR}/.env
	cd ${BUILD_DIR} && ./seckill