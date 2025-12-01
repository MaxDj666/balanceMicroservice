build:
	GOOS=linux GOARCH=amd64 go build -o bin/app-linux-amd64 main.go
	GOOS=darwin GOARCH=arm64 go build -o bin/app-darwin-arm64 main.go

run:
	go run main.go

up: build
	docker-compose up -d

down:
	docker-compose down --remove-orphans

clean:
	rm -rf ./bin
