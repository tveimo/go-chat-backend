GO_PROJECT_NAME := go-chat-backend

go_build:
	@echo "\n....Building $(GO_PROJECT_NAME)"
	go build -o ./bin/$(GO_PROJECT_NAME) `ls -1 *.go`

go_dep_install:
	@echo "\n....Installing dependencies for $(GO_PROJECT_NAME)...."
	go get -v .

go_run:
	@echo "\n....Running $(GO_PROJECT_NAME)...."
	CLIENT_ID=user@domain.org CLIENT_SECRET=mypassword ./bin/$(GO_PROJECT_NAME)

# Project rules
build:
	$(MAKE) go_dep_install
	$(MAKE) go_build

test:
	UNIT_TESTING=true CLIENT_ID=admin CLIENT_SECRET=mypassword go test -v -p 1
	rm -rf unit-test.sqlite

test-ff:
	go get github.com/stretchr/testify/assert
	UNIT_TESTING=true CLIENT_ID=admin CLIENT_SECRET=mypassword go test -v -p 1 --failfast

run:
ifeq ($(ENV), dev)
	$(MAKE) build
	CLIENT_ID=user@domain.org CLIENT_SECRET=mypassword ./bin/gin
else
	$(MAKE) go_build
	$(MAKE) go_run
endif

clean:
	rm -rf test.sqlite
	rm -rf unit-test.sqlite
	rm -rf ./pkg/*
	rm -rf ./bin/*

db:
	@echo "\n....Starting DB engine  ...."
	docker run -p 3306:3306 -v db:/var/lib/mysql -e MYSQL_ROOT_PASSWORD=changeme -e MYSQL_DATABASE=backend mysql:5.7

.PHONY: db

