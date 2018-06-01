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

run-pi:
	go build -o $(BINARY_NAME) $(CMD_MAIN)
	sudo systemctl restart pi-player

run-mac:
	go build -o $(BINARY_NAME) $(CMD_MAIN)
	./$(BINARY_NAME) --debug --test mac

run-linux:
	go build -o $(BINARY_NAME) $(CMD_MAIN)
	./$(BINARY_NAME) --debug --test linux

run-web:
	go build -o $(BINARY_NAME) $(CMD_MAIN)
	./$(BINARY_NAME) --debug --test web
