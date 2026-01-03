# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /build

# Copy go.mod and go.sum from parent golang directory
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy common libraries and hlsserver code
COPY common/ ./common/
COPY hlsserver/ ./hlsserver/

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o hlsserver ./hlsserver/cmd/main.go

# Runtime stage
FROM alpine:latest

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /build/hlsserver .

# Expose ports for token server and key server
EXPOSE 3100 3101

# Run the application
CMD ["./hlsserver"]
