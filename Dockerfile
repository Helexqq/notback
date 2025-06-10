FROM golang:1.24.4-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/backend ./cmd/server/main.go

FROM alpine:latest
WORKDIR /app

COPY --from=builder /app/backend .
COPY cmd/easter_egg.txt ./easter_egg.txt

EXPOSE 8080

ENTRYPOINT ["sh", "-c", "cat easter_egg.txt && ./backend"]
