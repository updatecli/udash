GOVERSION=$(shell go version)
export GOVERSION

DOCKER_BUILDKIT=1
export DOCKER_BUILDKIT

UDASH_DB_URI=postgres://udash:password@localhost:5432/udash?sslmode=disable
export UDASH_DB_URI

local_bin=./dist/updatecli_$(shell go env GOHOSTOS)_$(shell go env GOHOSTARCH)/updatecli

.PHONY: app.build
app.build: ## Build application localy
	go build -o bin/udash \
		-ldflags='-w -s -X "github.com/updatecli/udash/pkg/version.BuildTime=$(shell date)" -X "github.com/updatecli/udash/pkg/version.GoVersion=$(shell go version)" -X "github.com/updatecli/udash/pkg/version.Version=42"'

server.start: app.build ## Start application localy
	./bin/udash server start --debug

.PHONY: build
build: ## Build updatecli as a "dirty snapshot" (no tag, no release, but all OS/arch combinations)
	goreleaser build --snapshot --rm-dist

.PHONY: build.all
build.all: ## Build updatecli for "release" (tag or release and all OS/arch combinations)
	goreleaser --rm-dist --skip-publish

clean: ## Clean go test cache
	go clean -testcache

.PHONY: release ## Create a new updatecli release including packages
release: ## release.snapshot generate a snapshot release but do not published it (no tag, but all OS/arch combinations)
	goreleaser --rm-dist

.PHONY: release.snapshot ## Create a new snapshot release without publishing assets
release.snapshot: ## release.snapshot generate a snapshot release but do not published it (no tag, but all OS/arch combinations)
	goreleaser --snapshot --rm-dist --skip-publish

.PHONY: db 
db.reset: db.delete db.start ## Reset development database

.PHONY: db.connect
db.connect: ## Connect to development database
	docker exec -i -t udash-db-1 psql --username=udash --password udash

.PHONY: db.start
db.start: ## Start development database
	docker compose up -d db

.PHONY: db.delete
db.delete: ## Delete development database
	docker compose down db -v

.PHONY: help
help: ## Show this Makefile's help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'


.PHONY: Run application linting tests
lint: ## Execute the Golang's linters on updatecli's source code
	golangci-lint run
	
.PHONY: Run application tests
test: ## Execute the Golang's tests for updatecli
	go test ./... -race -coverprofile=coverage.txt -covermode=atomic

