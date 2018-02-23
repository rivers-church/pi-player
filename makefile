# variables
BINARY_NAME=pi-player
CMD_MAIN=cmd/piPlayer/main.go

build:
	go build -o $(BINARY_NAME) $(CMD_MAIN)

run-pi:
	go build -o $(BINARY_NAME) $(CMD_MAIN)
	sudo systemctl restart pi-player

run-mac:
	go build -o $(BINARY_NAME) $(CMD_MAIN)
	./$(BINARY_NAME) --debug --test mac

run-linux:
	go build -o $(BINARY_NAME) $(CMD_MAIN)
	./$(BINARY_NAME) --debug --test linux
	