FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY worker/go.mod worker/go.sum ./
RUN go mod download
COPY worker/ .
RUN CGO_ENABLED=0 GOOS=linux go build -o /worker .

FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY --from=builder /worker /worker
ENTRYPOINT ["/worker"]
