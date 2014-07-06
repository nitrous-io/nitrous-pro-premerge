NO_COLOR=\033[0m
OK_COLOR=\033[32;01m
ERROR_COLOR=\033[31;01m
WARN_COLOR=\033[33;01m
BIN_NAME=nitrousd
REVISION=$(shell git rev-parse --short HEAD)
BASE_VERSION=$(shell cat VERSION)
VERSION=$(BASE_VERSION)-$(REVISION)

all: build

image:
	@docker build -t registry.nitrous.io/nitrous .
.PHONY: image

go:
	@echo "$(OK_COLOR)==> Building revision $(VERSION)...$(NO_COLOR)"
	@script/build-go

assets:
	@echo "$(OK_COLOR)==> Building assets...$(NO_COLOR)"
	@script/build-assets

build: go assets

test:
	@echo "$(OK_COLOR)==> Testing...$(NO_COLOR)"
	@script/test

format:
	go fmt ./...

run: build
	@echo "$(OK_COLOR)==> Running...$(NO_COLOR)"
	@goop exec forego start

createdb:
	createdb -E UTF8 -T template0 -l en_US.utf8 nitrous_development || true
	su postgres -c "psql -d nitrous_development -c 'CREATE EXTENSION IF NOT EXISTS hstore;'" || true
	createdb -E UTF8 -T template0 -l en_US.utf8 nitrous_test || true
	su postgres -c "psql -d nitrous_test -c 'CREATE EXTENSION IF NOT EXISTS hstore;'" || true

dropdb:
	dropdb --if-exists nitrous_development
	dropdb --if-exists nitrous_migrations_test
	dropdb --if-exists nitrous_test

.PHONY: all go assets build format run test createdb dropdb
