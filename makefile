# variables
BINARY_NAME=pi-player
CMD_MAIN=main.go

define CROSS_COMPILE_LOOP
for GOOS in darwin linux; do \
	for GOARCH in 386 amd64 arm64; do \
		go build -v -o dist/$(BINARY_NAME)-$$GOOS-$$GOARCH; \
	done; \
done
endef

build:
	go build

build-cross:
	sudo docker run --rm -v "$$PWD":/usr/$(BINARY_NAME) -w /usr/$(BINARY_NAME) golang /bin/sh -c '$(CROSS_COMPILE_LOOP)'

# build with a docker container
docker:
	sudo docker run --rm -v "$$PWD":/usr/$(BINARY_NAME) -w /usr/$(BINARY_NAME) golang go build -v

upgrade:
	sudo systemctl stop pi-player
	git pull
	go build
	sudo systemctl start pi-player

run-pi: build
	sudo systemctl restart pi-player

run-mac: build
	./$(BINARY_NAME) --debug --test mac

run-linux: build
	./$(BINARY_NAME) --debug --test linux

run-web: build
	./$(BINARY_NAME) --debug --test web
