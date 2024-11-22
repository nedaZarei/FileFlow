FROM golang:1.23.2-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o consumer

FROM scratch
COPY --from=builder /app/consumer /app/consumer

COPY --from=builder /app/config/config.yaml /app/config.yaml

EXPOSE 8000

ENTRYPOINT ["/app/consumer"]