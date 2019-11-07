FROM golang:1.13-alpine AS builder

WORKDIR /app
COPY . .
RUN go get -v ./... && CGO_ENABLED=0 go build

FROM alpine:latest
COPY --from=builder /app/yamaha-openhab /yamaha-openhab
ENTRYPOINT ["/yamaha-openhab"]
