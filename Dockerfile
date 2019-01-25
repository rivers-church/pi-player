FROM golang:latest

WORKDIR /app
COPY . .

RUN go build
EXPOSE 8080

# TODO: Expose USB interface to read remote control imput

CMD ["pi-player"]