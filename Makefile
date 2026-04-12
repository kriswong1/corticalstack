.PHONY: ui-install ui-build ui-dev build dev clean

ui-install:
	cd ui && npm ci

ui-build: ui-install
	cd ui && npm run build

ui-dev:
	cd ui && npm run dev

build: ui-build
	go build -o cortical.exe ./cmd/cortical

dev:
	go run ./cmd/cortical

clean:
	rm -rf ui/node_modules internal/web/spa/dist/*
	rm -f cortical.exe
