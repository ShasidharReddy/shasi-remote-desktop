FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN cd relay-server && go build -o /relay-server .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
COPY --from=builder /relay-server /relay-server
EXPOSE 9090
CMD ["/relay-server"]
