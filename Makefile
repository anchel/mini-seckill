NAME=seckill
BUILD_DIR=build

run:
	go build -o ${BUILD_DIR}/seckill main.go
	cp .env ${BUILD_DIR}/.env
	cd ${BUILD_DIR} && ./seckill