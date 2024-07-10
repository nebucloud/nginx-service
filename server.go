package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/hashicorp/consul/api"
	"github.com/nebucloud/nginx-service/api/go/nginx/v1"
	"github.com/nebucloud/nginx-service/graphql"
	"github.com/nebucloud/nginx-service/resolver"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

type server struct {
	nginx.UnimplementedNginxConfigServiceServer
}

func (s *server) GetConfig(ctx context.Context, req *nginx.ConfigRequest) (*nginx.ConfigResponse, error) {
	// Implement logic to get config
	return &nginx.ConfigResponse{
		Success:    true,
		Message:    "Config retrieved successfully",
		ConfigData: "{\"example\": \"data\"}",
	}, nil
}

func (s *server) ApplyConfig(ctx context.Context, req *nginx.ConfigRequest) (*nginx.ConfigResponse, error) {
	// Implement logic to apply config
	return &nginx.ConfigResponse{
		Success: true,
		Message: "Config applied successfully",
	}, nil
}

func startGraphQLServer(port string, stopChan <-chan struct{}) error {
	srv := handler.NewDefaultServer(graphql.NewExecutableSchema(graphql.Config{Resolvers: &resolver.Resolver{}}))
	http.Handle("/", playground.Handler("GraphQL playground", "/query"))
	http.Handle("/query", srv)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	server := &http.Server{Addr: ":" + port}

	go func() {
		log.Printf("GraphQL server is running at http://localhost:%s/", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("GraphQL server error: %v", err)
		}
	}()

	<-stopChan
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return server.Shutdown(ctx)
}

func startGRPCServer(port string, stopChan <-chan struct{}) error {
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return err
	}
	grpcServer := grpc.NewServer()
	nginx.RegisterNginxConfigServiceServer(grpcServer, &server{})
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

	go func() {
		log.Printf("gRPC server listening on port %s", port)
		if err := grpcServer.Serve(lis); err != nil {
			log.Printf("gRPC server error: %v", err)
		}
	}()

	<-stopChan
	grpcServer.GracefulStop()
	return nil
}

func registerServiceWithConsul(serviceID, serviceName, serviceAddress string, servicePort int, tags []string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered from panic in Consul registration for %s: %v", serviceName, r)
		}
	}()

	maxRetries := 5
	retryDelay := 5 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("Recovered from panic in Consul client creation for %s (attempt %d/%d): %v", serviceName, attempt, maxRetries, r)
				}
			}()

			config := api.DefaultConfig()
			config.HttpClient = &http.Client{Timeout: 5 * time.Second}

			consulClient, err := api.NewClient(config)
			if err != nil {
				log.Printf("Error creating Consul client for %s (attempt %d/%d): %v", serviceName, attempt, maxRetries, err)
				return
			}

			registration := &api.AgentServiceRegistration{
				ID:      serviceID,
				Name:    serviceName,
				Port:    servicePort,
				Address: serviceAddress,
				Tags:    tags,
				Check: &api.AgentServiceCheck{
					HTTP:                           fmt.Sprintf("http://%s:%d/health", serviceAddress, servicePort),
					Interval:                       "10s",
					Timeout:                        "5s",
					DeregisterCriticalServiceAfter: "1m",
				},
			}

			err = consulClient.Agent().ServiceRegister(registration)
			if err != nil {
				log.Printf("Failed to register %s with Consul (attempt %d/%d): %v", serviceName, attempt, maxRetries, err)
				return
			}

			log.Printf("Successfully registered %s with Consul", serviceName)
		}()

		if attempt < maxRetries {
			time.Sleep(retryDelay)
		}
	}

	log.Printf("Completed Consul registration attempts for %s", serviceName)
}

func main() {
	graphqlPort := getEnvOrDefault("GRAPHQL_PORT", "4001")
	grpcPort := getEnvOrDefault("GRPC_PORT", "50051")
	serviceAddress := getEnvOrDefault("SERVICE_ADDRESS", "localhost")
	consulEnabled := getEnvOrDefault("CONSUL_ENABLED", "true") == "true"

	var wg sync.WaitGroup
	wg.Add(2)

	stopChan := make(chan struct{})
	errorChan := make(chan error, 2)

	// Attempt to register services with Consul if enabled
	if consulEnabled {
		go func() {
			registerServiceWithConsul("graphql-service", "GraphQL", serviceAddress, mustAtoi(graphqlPort), []string{"graphql", "http"})
			registerServiceWithConsul("grpc-service", "gRPC", serviceAddress, mustAtoi(grpcPort), []string{"grpc"})
		}()
	} else {
		log.Println("Consul registration is disabled")
	}

	// Start GraphQL server
	go func() {
		defer wg.Done()
		if err := startGraphQLServer(graphqlPort, stopChan); err != nil {
			errorChan <- err
		}
	}()

	// Start gRPC server
	go func() {
		defer wg.Done()
		if err := startGRPCServer(grpcPort, stopChan); err != nil {
			errorChan <- err
		}
	}()

	// Wait for interrupt signal or errors
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case <-sigChan:
		log.Println("Received interrupt signal. Shutting down servers...")
	case err := <-errorChan:
		log.Printf("Received error: %v. Shutting down servers...", err)
	}

	close(stopChan)
	wg.Wait()
	log.Println("Servers stopped gracefully.")
}

func getEnvOrDefault(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func mustAtoi(s string) int {
	i, err := strconv.Atoi(s)
	if err != nil {
		panic(err)
	}
	return i
}
