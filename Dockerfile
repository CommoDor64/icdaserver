FROM golang:1.23-alpine

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN go build -o server .

CMD ["sh", "-c", "go run scripts/main.go && ./server"]
