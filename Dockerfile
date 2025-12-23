FROM golang:1.25 AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -v -o server .
FROM alpine:latest
WORKDIR /app/

COPY --from=builder /app/server  /app/server

VOLUME /app/storage /app/output
EXPOSE 8080
CMD ["/app/server"]
