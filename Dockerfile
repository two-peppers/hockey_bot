FROM --platform=linux/amd64 golang:1.25-bookworm AS builder

ENV CGO_ENABLED=0
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -ldflags '-s -w' -o hockey_bot ./cmd/bot

FROM --platform=linux/amd64 debian:bookworm-slim
RUN apt-get update && apt-get install -y ca-certificates tzdata && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=builder /app/hockey_bot .
COPY .env .
CMD ["./hockey_bot"]