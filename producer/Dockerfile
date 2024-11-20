FROM golang:1.23.2-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o producer

FROM scratch
COPY --from=builder /app/producer /app/producer

EXPOSE 8000

ENTRYPOINT ["/app/producer"]