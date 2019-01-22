# variables
BINARY_NAME=pi-player
CMD_MAIN=main.go

build:
	go build

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
