.PHONY: build test frontend verify
build:
	go build -buildvcs=false -trimpath -o bin/mmp-api ./cmd/api
	go build -buildvcs=false -trimpath -o bin/create-admin ./cmd/create-admin
	go build -buildvcs=false -trimpath -o bin/create-user ./cmd/create-user
test:
	go test -race ./...
	go vet ./...
frontend:
	cd web && npm ci && npm run build
verify: test build frontend
