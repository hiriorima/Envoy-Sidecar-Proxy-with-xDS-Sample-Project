package main

import (
	"context"
	"time"

	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	router "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	accesslog "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/file/v3"
	accesslog_config "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	ClusterName         = "spring_boot_cluster"
	ExternalClusterName = "nginx_cluster"
	RouteName           = "local_route"
	EgressRouteName     = "egress_route"
	ListenerName        = "ingress_listener"
	EgressListenerName  = "egress_listener"
	ListenerPort        = 8080
	EgressListenerPort  = 8081
)

func makeCluster(clusterName string, address string, port uint32) *cluster.Cluster {
	outlierDetection := &cluster.OutlierDetection{
		Consecutive_5Xx:                    wrapperspb.UInt32(3),
		ConsecutiveGatewayFailure:          wrapperspb.UInt32(3),
		Interval:                           durationpb.New(30 * time.Second),
		BaseEjectionTime:                   durationpb.New(30 * time.Second),
		MaxEjectionPercent:                 wrapperspb.UInt32(50),
	}

	if clusterName == ExternalClusterName {
		outlierDetection.Consecutive_5Xx = wrapperspb.UInt32(5)
		outlierDetection.ConsecutiveGatewayFailure = wrapperspb.UInt32(5)
	}

	c := &cluster.Cluster{
		Name:                 clusterName,
		ConnectTimeout:       durationpb.New(250 * time.Millisecond),
		ClusterDiscoveryType: &cluster.Cluster_Type{Type: cluster.Cluster_LOGICAL_DNS},
		DnsLookupFamily:      cluster.Cluster_V4_ONLY,
		LbPolicy:             cluster.Cluster_ROUND_ROBIN,
		OutlierDetection:     outlierDetection,
		LoadAssignment: &endpoint.ClusterLoadAssignment{
			ClusterName: clusterName,
			Endpoints: []*endpoint.LocalityLbEndpoints{{
				LbEndpoints: []*endpoint.LbEndpoint{{
					HostIdentifier: &endpoint.LbEndpoint_Endpoint{
						Endpoint: &endpoint.Endpoint{
							Address: &core.Address{
								Address: &core.Address_SocketAddress{
									SocketAddress: &core.SocketAddress{
										Protocol: core.SocketAddress_TCP,
										Address:  address,
										PortSpecifier: &core.SocketAddress_PortValue{
											PortValue: port,
										},
									},
								},
							},
						},
					},
				}},
			}},
		},
	}

	// Add circuit breakers for external cluster
	if clusterName == ExternalClusterName {
		c.CircuitBreakers = &cluster.CircuitBreakers{
			Thresholds: []*cluster.CircuitBreakers_Thresholds{{
				Priority:           core.RoutingPriority_DEFAULT,
				MaxConnections:     wrapperspb.UInt32(100),
				MaxPendingRequests: wrapperspb.UInt32(50),
				MaxRequests:        wrapperspb.UInt32(200),
				MaxRetries:         wrapperspb.UInt32(3),
			}},
		}
	}

	return c
}

func makeRoute(routeName string, clusterName string, isEgress bool) *route.RouteConfiguration {
	retryPolicy := &route.RetryPolicy{
		RetryOn:       "5xx,reset,connect-failure,refused-stream",
		NumRetries:    wrapperspb.UInt32(3),
		PerTryTimeout: durationpb.New(5 * time.Second),
		RetryBackOff: &route.RetryPolicy_RetryBackOff{
			BaseInterval: durationpb.New(500 * time.Millisecond),
			MaxInterval:  durationpb.New(10 * time.Second),
		},
	}

	prefix := "/"
	if isEgress {
		prefix = "/external"
		retryPolicy.PerTryTimeout = durationpb.New(3 * time.Second)
		retryPolicy.RetryBackOff.BaseInterval = durationpb.New(250 * time.Millisecond)
		retryPolicy.RetryBackOff.MaxInterval = durationpb.New(5 * time.Second)
	}

	return &route.RouteConfiguration{
		Name: routeName,
		VirtualHosts: []*route.VirtualHost{{
			Name:    "local_service",
			Domains: []string{"*"},
			Routes: []*route.Route{{
				Match: &route.RouteMatch{
					PathSpecifier: &route.RouteMatch_Prefix{
						Prefix: prefix,
					},
				},
				Action: &route.Route_Route{
					Route: &route.RouteAction{
						ClusterSpecifier: &route.RouteAction_Cluster{
							Cluster: clusterName,
						},
						RetryPolicy: retryPolicy,
					},
				},
			}},
		}},
	}
}

func makeHTTPListener(listenerName string, routeName string, port uint32) *listener.Listener {
	// JSON Access log configuration
	jsonFormat, _ := structpb.NewStruct(map[string]interface{}{
		"timestamp":              "%START_TIME%",
		"method":                 "%REQ(:METHOD)%",
		"path":                   "%REQ(X-ENVOY-ORIGINAL-PATH?:PATH)%",
		"protocol":               "%PROTOCOL%",
		"response_code":          "%RESPONSE_CODE%",
		"response_flags":         "%RESPONSE_FLAGS%",
		"bytes_received":         "%BYTES_RECEIVED%",
		"bytes_sent":             "%BYTES_SENT%",
		"duration":               "%DURATION%",
		"upstream_service_time":  "%RESP(X-ENVOY-UPSTREAM-SERVICE-TIME)%",
		"x_forwarded_for":        "%REQ(X-FORWARDED-FOR)%",
		"user_agent":             "%REQ(USER-AGENT)%",
		"request_id":             "%REQ(X-REQUEST-ID)%",
		"authority":              "%REQ(:AUTHORITY)%",
		"upstream_host":          "%UPSTREAM_HOST%",
	})

	accessLogConfig := &accesslog.FileAccessLog{
		Path: "/dev/stdout",
		AccessLogFormat: &accesslog.FileAccessLog_JsonFormat{
			JsonFormat: jsonFormat,
		},
	}

	accessLogAny, _ := anypb.New(accessLogConfig)
	routerConfig, _ := anypb.New(&router.Router{})

	manager := &hcm.HttpConnectionManager{
		CodecType:  hcm.HttpConnectionManager_AUTO,
		StatPrefix: "ingress_http",
		RouteSpecifier: &hcm.HttpConnectionManager_RouteConfig{
			RouteConfig: makeRoute(routeName, ClusterName, false),
		},
		HttpFilters: []*hcm.HttpFilter{{
			Name: wellknown.Router,
			ConfigType: &hcm.HttpFilter_TypedConfig{
				TypedConfig: routerConfig,
			},
		}},
		AccessLog: []*accesslog_config.AccessLog{{
			Name: wellknown.FileAccessLog,
			ConfigType: &accesslog_config.AccessLog_TypedConfig{
				TypedConfig: accessLogAny,
			},
		}},
	}

	if listenerName == EgressListenerName {
		manager.StatPrefix = "egress_http"
		manager.RouteSpecifier = &hcm.HttpConnectionManager_RouteConfig{
			RouteConfig: makeRoute(EgressRouteName, ExternalClusterName, true),
		}
	}

	pbst, _ := anypb.New(manager)

	return &listener.Listener{
		Name: listenerName,
		Address: &core.Address{
			Address: &core.Address_SocketAddress{
				SocketAddress: &core.SocketAddress{
					Protocol: core.SocketAddress_TCP,
					Address:  "0.0.0.0",
					PortSpecifier: &core.SocketAddress_PortValue{
						PortValue: port,
					},
				},
			},
		},
		FilterChains: []*listener.FilterChain{{
			Filters: []*listener.Filter{{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &listener.Filter_TypedConfig{
					TypedConfig: pbst,
				},
			}},
		}},
	}
}

func setSnapshot(snapshotCache cache.SnapshotCache) error {
	return setSnapshotWithParams(snapshotCache, "1", "spring_app", 8080, "nginx_external_api", 80)
}

func setSnapshotWithParams(snapshotCache cache.SnapshotCache, version string, springHost string, springPort uint32, nginxHost string, nginxPort uint32) error {
	snap, _ := cache.NewSnapshot(version,
		map[resource.Type][]types.Resource{
			resource.ClusterType: {
				makeCluster(ClusterName, springHost, springPort),
				makeCluster(ExternalClusterName, nginxHost, nginxPort),
			},
			resource.RouteType: {
				makeRoute(RouteName, ClusterName, false),
				makeRoute(EgressRouteName, ExternalClusterName, true),
			},
			resource.ListenerType: {
				makeHTTPListener(ListenerName, RouteName, ListenerPort),
				makeHTTPListener(EgressListenerName, EgressRouteName, EgressListenerPort),
			},
		},
	)

	return snapshotCache.SetSnapshot(context.Background(), "test-node", snap)
}

