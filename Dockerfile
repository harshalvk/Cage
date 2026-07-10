FROM golang:1.25-alpine AS builder

WORKDIR /app

# cache dependencies separately from source changes
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /cage ./cmd/cage

FROM alpine:latest

RUN apk add --no-cache ca-certificates   docker-cli

WORKDIR /app

COPY --from=builder /cage .
COPY migrations ./migrations

EXPOSE 8080

CMD ["./cage"]