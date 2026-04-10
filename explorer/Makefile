.PHONY: frontend build run clean

frontend:
	cd frontend && bun install && bun run build.ts

build: frontend
	go build -o bin/explorer ./cmd/explorer/

run: build
	cd frontend && ../bin/explorer --grpc-addr localhost:5500 --http-addr :8080

clean:
	rm -rf bin frontend/dist frontend/node_modules
