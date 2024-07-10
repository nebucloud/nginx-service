# Use ChainGuard's Go image for building the Go application
FROM cgr.dev/chainguard/go AS builder

# Set the working directory in the container
WORKDIR /app

# Copy the go mod and sum files first to leverage Docker cache
COPY go.mod go.sum ./

# Download all dependencies. Dependencies will be cached if the go.* files are not changed
RUN go mod download

# Copy the rest of the application code
COPY . .

# Build the project. Replace pkg/nginx_service with the directory containing the main file
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o nginx-service .

# Use ChainGuard's glibc-dynamic image for the runtime environment
FROM cgr.dev/chainguard/glibc-dynamic

# Copy the built binary from the builder stage to the runtime container
COPY --from=builder /app/nginx-service /usr/bin/nginx-service

# Expose the ports used by the gRPC and GraphQL servers
EXPOSE 50051
EXPOSE 4001

# Command to run the application
CMD ["/usr/bin/nginx-service"]