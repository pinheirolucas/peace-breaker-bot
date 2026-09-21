.PHONY = build run

PACKAGE_NAME = github.com/pinheirolucas/peace-breaker-bot
BIN = ./bin/peace-breaker-bot

build:
	go build -o ${BIN} ${PACKAGE_NAME}

run: build
	${BIN}

test:
	go test -race -timeout 90s ./...

lint:
	golangci-lint run ./...

cover:
	go test -coverprofile cp.out ./...
	go tool cover -html=cp.out

.PHONY = clean
clean:
	go clean
	rm -rf ./bin
	rm --force cp.out
	rm --force nohup.out
