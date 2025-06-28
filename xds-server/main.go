package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	clusterservice "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	discoverygrpc "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	endpointservice "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	listenerservice "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	routeservice "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/server/v3"
)

const (
	grpcKeepaliveTime        = 30 * time.Second
	grpcKeepaliveTimeout     = 5 * time.Second
	grpcKeepaliveMinTime     = 30 * time.Second
	grpcMaxConcurrentStreams = 1000000
)

var (
	snapshotCache cache.SnapshotCache
	versionCounter int64 = 1
)

func updateConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters for dynamic configuration
	springPort := r.URL.Query().Get("spring_port")
	nginxPort := r.URL.Query().Get("nginx_port")
	springHost := r.URL.Query().Get("spring_host")
	nginxHost := r.URL.Query().Get("nginx_host")

	// Set defaults if not provided
	if springPort == "" {
		springPort = "8080"
	}
	if nginxPort == "" {
		nginxPort = "80"
	}
	if springHost == "" {
		springHost = "spring_app"
	}
	if nginxHost == "" {
		nginxHost = "nginx_external_api"
	}

	// Convert ports to uint32
	springPortUint, err := strconv.ParseUint(springPort, 10, 32)
	if err != nil {
		http.Error(w, "Invalid spring_port", http.StatusBadRequest)
		return
	}
	nginxPortUint, err := strconv.ParseUint(nginxPort, 10, 32)
	if err != nil {
		http.Error(w, "Invalid nginx_port", http.StatusBadRequest)
		return
	}

	// Increment version for cache consistency
	versionCounter++
	version := strconv.FormatInt(versionCounter, 10)

	// Update snapshot with new configuration
	if err := setSnapshotWithParams(snapshotCache, version, springHost, uint32(springPortUint), nginxHost, uint32(nginxPortUint)); err != nil {
		log.Printf("Failed to update snapshot: %v", err)
		http.Error(w, "Failed to update configuration", http.StatusInternalServerError)
		return
	}

	log.Printf("Configuration updated: Spring=%s:%s, Nginx=%s:%s, Version=%s", 
		springHost, springPort, nginxHost, nginxPort, version)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Configuration updated successfully"))
}

func main() {
	// Create a cache
	snapshotCache = cache.NewSnapshotCache(false, cache.IDHash{}, nil)

	// Create server
	srv := server.NewServer(context.Background(), snapshotCache, nil)

	// gRPC server
	var grpcOptions []grpc.ServerOption
	grpcOptions = append(grpcOptions,
		grpc.MaxConcurrentStreams(grpcMaxConcurrentStreams),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    grpcKeepaliveTime,
			Timeout: grpcKeepaliveTimeout,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             grpcKeepaliveMinTime,
			PermitWithoutStream: true,
		}),
	)
	grpcServer := grpc.NewServer(grpcOptions...)

	// Register services
	discoverygrpc.RegisterAggregatedDiscoveryServiceServer(grpcServer, srv)
	endpointservice.RegisterEndpointDiscoveryServiceServer(grpcServer, srv)
	clusterservice.RegisterClusterDiscoveryServiceServer(grpcServer, srv)
	routeservice.RegisterRouteDiscoveryServiceServer(grpcServer, srv)
	listenerservice.RegisterListenerDiscoveryServiceServer(grpcServer, srv)

	// Set initial configuration
	if err := setSnapshot(snapshotCache); err != nil {
		log.Fatalf("Failed to set snapshot: %v", err)
	}

	// Start HTTP server for configuration updates
	http.HandleFunc("/update-config", updateConfig)
	go func() {
		log.Printf("HTTP API server listening on :8090")
		if err := http.ListenAndServe(":8090", nil); err != nil {
			log.Fatalf("Failed to start HTTP server: %v", err)
		}
	}()

	// Start gRPC server
	lis, err := net.Listen("tcp", ":18000")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	log.Printf("xDS server listening on %s", lis.Addr())
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}