# variables
BINARY_NAME=pi-player
CMD_MAIN=cmd/piplayer/main.go

build:
	go build -o $(BINARY_NAME) $(CMD_MAIN)

upgrade:
	sudo systemctl stop pi-player
	git pull
	go build -o $(BINARY_NAME) $(CMD_MAIN)
	sudo systemctl start pi-player

run-pi: build
	sudo systemctl restart pi-player

run-mac: build
	./$(BINARY_NAME) --debug --test mac

run-linux: build
	./$(BINARY_NAME) --debug --test linux

run-web: build
	./$(BINARY_NAME) --debug --test web
