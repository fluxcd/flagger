// proto: https://github.com/istio/api/blob/master/networking/v1alpha3/destination_rule.pb.go
package v1alpha3

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// DestinationRule
type DestinationRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              DestinationRuleSpec `json:"spec"`
}

// DestinationRule defines policies that apply to traffic intended for a
// service after routing has occurred. These rules specify configuration
// for load balancing, connection pool size from the sidecar, and outlier
// detection settings to detect and evict unhealthy hosts from the load
// balancing pool. For example, a simple load balancing policy for the
// ratings service would look as follows:
//
// apiVersion: networking.istio.io/v1alpha3
// kind: DestinationRule
// metadata:
//
//	name: bookinfo-ratings
//
// spec:
//
//	host: ratings.prod.svc.cluster.local
//	trafficPolicy:
//	  loadBalancer:
//	    simple: LEAST_CONN
//
// Version specific policies can be specified by defining a named
// subset and overriding the settings specified at the service level. The
// following rule uses a round robin load balancing policy for all traffic
// going to a subset named testversion that is composed of endpoints (e.g.,
// pods) with labels (version:v3).
//
// apiVersion: networking.istio.io/v1alpha3
// kind: DestinationRule
// metadata:
//
//	name: bookinfo-ratings
//
// spec:
//
//	host: ratings.prod.svc.cluster.local
//	trafficPolicy:
//	  loadBalancer:
//	    simple: LEAST_CONN
//	subsets:
//	- name: testversion
//	  labels:
//	    version: v3
//	  trafficPolicy:
//	    loadBalancer:
//	      simple: ROUND_ROBIN
//
// **Note:** Policies specified for subsets will not take effect until
// a route rule explicitly sends traffic to this subset.
//
// Traffic policies can be customized to specific ports as well. The
// following rule uses the least connection load balancing policy for all
// traffic to port 80, while uses a round robin load balancing setting for
// traffic to the port 9080.
//
// apiVersion: networking.istio.io/v1alpha3
// kind: DestinationRule
// metadata:
//
//	name: bookinfo-ratings-port
//
// spec:
//
//	host: ratings.prod.svc.cluster.local
//	trafficPolicy: # Apply to all ports
//	  portLevelSettings:
//	  - port:
//	      number: 80
//	    loadBalancer:
//	      simple: LEAST_CONN
//	  - port:
//	      number: 9080
//	    loadBalancer:
//	      simple: ROUND_ROBIN
type DestinationRuleSpec struct {
	// REQUIRED. The name of a service from the service registry. Service
	// names are looked up from the platform's service registry (e.g.,
	// Kubernetes services, Consul services, etc.) and from the hosts
	// declared by [ServiceEntries](#ServiceEntry). Rules defined for
	// services that do not exist in the service registry will be ignored.
	//
	// *Note for Kubernetes users*: When short names are used (e.g. "reviews"
	// instead of "reviews.default.svc.cluster.local"), Istio will interpret
	// the short name based on the namespace of the rule, not the service. A
	// rule in the "default" namespace containing a host "reviews will be
	// interpreted as "reviews.default.svc.cluster.local", irrespective of
	// the actual namespace associated with the reviews service. _To avoid
	// potential misconfigurations, it is recommended to always use fully
	// qualified domain names over short names._
	//
	// Note that the host field applies to both HTTP and TCP services.
	Host string `json:"host"`

	// Traffic policies to apply (load balancing policy, connection pool
	// sizes, outlier detection).
	TrafficPolicy *TrafficPolicy `json:"trafficPolicy,omitempty"`

	// One or more named sets that represent individual versions of a
	// service. Traffic policies can be overridden at subset level.
	Subsets []Subset `json:"subsets,omitempty"`
}

// Traffic policies to apply for a specific destination, across all
// destination ports. See DestinationRule for examples.
type TrafficPolicy struct {

	// Settings controlling the load balancer algorithms.
	LoadBalancer *LoadBalancerSettings `json:"loadBalancer,omitempty"`

	// Settings controlling the volume of connections to an upstream service
	ConnectionPool *ConnectionPoolSettings `json:"connectionPool,omitempty"`

	// Settings controlling eviction of unhealthy hosts from the load balancing pool
	OutlierDetection *OutlierDetection `json:"outlierDetection,omitempty"`

	// TLS related settings for connections to the upstream service.
	TLS *TLSSettings `json:"tls,omitempty"`

	// Traffic policies specific to individual ports. Note that port level
	// settings will override the destination-level settings. Traffic
	// settings specified at the destination-level will not be inherited when
	// overridden by port-level settings, i.e. default values will be applied
	// to fields omitted in port-level traffic policies.
	PortLevelSettings []PortTrafficPolicy `json:"portLevelSettings,omitempty"`
}

// Traffic policies that apply to specific ports of the service
type PortTrafficPolicy struct {
	// Specifies the port name or number of a port on the destination service
	// on which this policy is being applied.
	//
	// Names must comply with DNS label syntax (rfc1035) and therefore cannot
	// collide with numbers. If there are multiple ports on a service with
	// the same protocol the names should be of the form <protocol-name>-<DNS
	// label>.
	Port PortSelector `json:"port"`

	// Settings controlling the load balancer algorithms.
	LoadBalancer *LoadBalancerSettings `json:"loadBalancer,omitempty"`

	// Settings controlling the volume of connections to an upstream service
	ConnectionPool *ConnectionPoolSettings `json:"connectionPool,omitempty"`

	// Settings controlling eviction of unhealthy hosts from the load balancing pool
	OutlierDetection *OutlierDetection `json:"outlierDetection,omitempty"`

	// TLS related settings for connections to the upstream service.
	TLS *TLSSettings `json:"tls,omitempty"`
}

// A subset of endpoints of a service. Subsets can be used for scenarios
// like A/B testing, or routing to a specific version of a service. Refer
// to [VirtualService](#VirtualService) documentation for examples of using
// subsets in these scenarios. In addition, traffic policies defined at the
// service-level can be overridden at a subset-level. The following rule
// uses a round robin load balancing policy for all traffic going to a
// subset named testversion that is composed of endpoints (e.g., pods) with
// labels (version:v3).
//
// apiVersion: networking.istio.io/v1alpha3
// kind: DestinationRule
// metadata:
//
//	name: bookinfo-ratings
//
// spec:
//
//	host: ratings.prod.svc.cluster.local
//	trafficPolicy:
//	  loadBalancer:
//	    simple: LEAST_CONN
//	subsets:
//	- name: testversion
//	  labels:
//	    version: v3
//	  trafficPolicy:
//	    loadBalancer:
//	      simple: ROUND_ROBIN
//
// **Note:** Policies specified for subsets will not take effect until
// a route rule explicitly sends traffic to this subset.
type Subset struct {
	// REQUIRED. Name of the subset. The service name and the subset name can
	// be used for traffic splitting in a route rule.
	Name string `json:"name"`

	// REQUIRED. Labels apply a filter over the endpoints of a service in the
	// service registry. See route rules for examples of usage.
	Labels map[string]string `json:"labels"`

	// Traffic policies that apply to this subset. Subsets inherit the
	// traffic policies specified at the DestinationRule level. Settings
	// specified at the subset level will override the corresponding settings
	// specified at the DestinationRule level.
	TrafficPolicy *TrafficPolicy `json:"trafficPolicy,omitempty"`
}

// Load balancing policies to apply for a specific destination. See Envoy's
// load balancing
// [documentation](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/load_balancing.html)
// for more details.
//
// For example, the following rule uses a round robin load balancing policy
// for all traffic going to the ratings service.
//
// apiVersion: networking.istio.io/v1alpha3
// kind: DestinationRule
// metadata:
//
//	name: bookinfo-ratings
//
// spec:
//
//	host: ratings.prod.svc.cluster.local
//	trafficPolicy:
//	  loadBalancer:
//	    simple: ROUND_ROBIN
//
// The following example sets up sticky sessions for the ratings service
// hashing-based load balancer for the same ratings service using the
// the User cookie as the hash key.
//
//	apiVersion: networking.istio.io/v1alpha3
//	kind: DestinationRule
//	metadata:
//	  name: bookinfo-ratings
//	spec:
//	  host: ratings.prod.svc.cluster.local
//	  trafficPolicy:
//	    loadBalancer:
//	      consistentHash:
//	        httpCookie:
//	          name: user
//	          ttl: 0s
type LoadBalancerSettings struct {
	// It is required to specify exactly one of the fields:
	// Simple or ConsistentHash
	Simple         SimpleLB          `json:"simple,omitempty"`
	ConsistentHash *ConsistentHashLB `json:"consistentHash,omitempty"`
	// Locality load balancer settings, this will override mesh wide settings in entirety, meaning no merging would be performed
	// between this object and the object one in MeshConfig
	LocalityLbSetting *LocalityLbSetting `json:"localityLbSetting,omitempty"`
}

// Locality-weighted load balancing allows administrators to control the
// distribution of traffic to endpoints based on the localities of where the
// traffic originates and where it will terminate. These localities are
// specified using arbitrary labels that designate a hierarchy of localities in
// {region}/{zone}/{sub-zone} form. For additional detail refer to
// [Locality Weight](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/upstream/load_balancing/locality_weight)
// The following example shows how to setup locality weights mesh-wide.
//
// Given a mesh with workloads and their service deployed to "us-west/zone1/*"
// and "us-west/zone2/*". This example specifies that when traffic accessing a
// service originates from workloads in "us-west/zone1/*", 80% of the traffic
// will be sent to endpoints in "us-west/zone1/*", i.e the same zone, and the
// remaining 20% will go to endpoints in "us-west/zone2/*". This setup is
// intended to favor routing traffic to endpoints in the same locality.
// A similar setting is specified for traffic originating in "us-west/zone2/*".
//
// ```yaml
//
//	distribute:
//	  - from: us-west/zone1/*
//	    to:
//	      "us-west/zone1/*": 80
//	      "us-west/zone2/*": 20
//	  - from: us-west/zone2/*
//	    to:
//	      "us-west/zone1/*": 20
//	      "us-west/zone2/*": 80
//
// ```
//
// If the goal of the operator is not to distribute load across zones and
// regions but rather to restrict the regionality of failover to meet other
// operational requirements an operator can set a 'failover' policy instead of
// a 'distribute' policy.
//
// The following example sets up a locality failover policy for regions.
// Assume a service resides in zones within us-east, us-west & eu-west
// this example specifies that when endpoints within us-east become unhealthy
// traffic should failover to endpoints in any zone or sub-zone within eu-west
// and similarly us-west should failover to us-east.
//
// ```yaml
//
//	failover:
//	  - from: us-east
//	    to: eu-west
//	  - from: us-west
//	    to: us-east
//
// ```
// Locality load balancing settings.
type LocalityLbSetting struct {
	// Explicitly specify loadbalancing weight across different zones and geographical locations.
	// Refer to [Locality weighted load balancing](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/upstream/load_balancing/locality_weight)
	// If empty, the locality weight is set according to the endpoints number within it.
	Distribute []Distribute `json:"distribute,omitempty"`
	// Optional: only one of distribute, failover or failoverPriority can be set.
	// Explicitly specify the region traffic will land on when endpoints in local region becomes unhealthy.
	// Should be used together with OutlierDetection to detect unhealthy endpoints.
	// Note: if no OutlierDetection specified, this will not take effect.
	Failover []Failover `json:"failover,omitempty"`
	// failoverPriority is an ordered list of labels used to sort endpoints to do priority based load balancing.
	// This is to support traffic failover across different groups of endpoints.
	// Suppose there are total N labels specified:
	//
	// 1. Endpoints matching all N labels with the client proxy have priority P(0) i.e. the highest priority.
	// 2. Endpoints matching the first N-1 labels with the client proxy have priority P(1) i.e. second highest priority.
	// 3. By extension of this logic, endpoints matching only the first label with the client proxy has priority P(N-1) i.e. second lowest priority.
	// 4. All the other endpoints have priority P(N) i.e. lowest priority.
	//
	// Note: For a label to be considered for match, the previous labels must match, i.e. nth label would be considered matched only if first n-1 labels match.
	//
	// It can be any label specified on both client and server workloads.
	// The following labels which have special semantic meaning are also supported:
	//
	//   - `topology.istio.io/network` is used to match the network metadata of an endpoint, which can be specified by pod/namespace label `topology.istio.io/network`, sidecar env `ISTIO_META_NETWORK` or MeshNetworks.
	//   - `topology.istio.io/cluster` is used to match the clusterID of an endpoint, which can be specified by pod label `topology.istio.io/cluster` or pod env `ISTIO_META_CLUSTER_ID`.
	//   - `topology.kubernetes.io/region` is used to match the region metadata of an endpoint, which maps to Kubernetes node label `topology.kubernetes.io/region` or the deprecated label `failure-domain.beta.kubernetes.io/region`.
	//   - `topology.kubernetes.io/zone` is used to match the zone metadata of an endpoint, which maps to Kubernetes node label `topology.kubernetes.io/zone` or the deprecated label `failure-domain.beta.kubernetes.io/zone`.
	//   - `topology.istio.io/subzone` is used to match the subzone metadata of an endpoint, which maps to Istio node label `topology.istio.io/subzone`.
	//
	// The below topology config indicates the following priority levels:
	//
	// ```yaml
	// failoverPriority:
	// - "topology.istio.io/network"
	// - "topology.kubernetes.io/region"
	// - "topology.kubernetes.io/zone"
	// - "topology.istio.io/subzone"
	// ```
	//
	// 1. endpoints match same [network, region, zone, subzone] label with the client proxy have the highest priority.
	// 2. endpoints have same [network, region, zone] label but different [subzone] label with the client proxy have the second highest priority.
	// 3. endpoints have same [network, region] label but different [zone] label with the client proxy have the third highest priority.
	// 4. endpoints have same [network] but different [region] labels with the client proxy have the fourth highest priority.
	// 5. all the other endpoints have the same lowest priority.
	//
	// Optional: only one of distribute, failover or failoverPriority can be set.
	// And it should be used together with `OutlierDetection` to detect unhealthy endpoints, otherwise has no effect.
	FailoverPriority []string `json:"failover_priority,omitempty"`
	// enable locality load balancing, this is DestinationRule-level and will override mesh wide settings in entirety.
	// e.g. true means that turn on locality load balancing for this DestinationRule no matter what mesh wide settings is.
	Enabled bool `json:"enabled,omitempty"`
}

// Describes how traffic originating in the 'from' zone or sub-zone is
// distributed over a set of 'to' zones. Syntax for specifying a zone is
// {region}/{zone}/{sub-zone} and terminal wildcards are allowed on any
// segment of the specification. Examples:
//
// `*` - matches all localities
//
// `us-west/*` - all zones and sub-zones within the us-west region
//
// `us-west/zone-1/*` - all sub-zones within us-west/zone-1
type Distribute struct {
	// Originating locality, '/' separated, e.g. 'region/zone/sub_zone'.
	From string `json:"from,omitempty"`
	// Map of upstream localities to traffic distribution weights. The sum of
	// all weights should be 100. Any locality not present will
	// receive no traffic.
	To map[string]uint32 `json:"to,omitempty"`
}

// Specify the traffic failover policy across regions. Since zone and sub-zone
// failover is supported by default this only needs to be specified for
// regions when the operator needs to constrain traffic failover so that
// the default behavior of failing over to any endpoint globally does not
// apply. This is useful when failing over traffic across regions would not
// improve service health or may need to be restricted for other reasons
// like regulatory controls.
type Failover struct {
	// Originating region.
	From string `json:"from,omitempty"`
	// Destination region the traffic will fail over to when endpoints in
	// the 'from' region becomes unhealthy.
	To string `json:"to,omitempty"`
}

// Standard load balancing algorithms that require no tuning.
type SimpleLB string

const (
	// Round Robin policy. Default
	SimpleLBRoundRobin SimpleLB = "ROUND_ROBIN"

	// The least request load balancer uses an O(1) algorithm which selects
	// two random healthy hosts and picks the host which has fewer active
	// requests.
	SimpleLBLeastConn SimpleLB = "LEAST_CONN"

	// The random load balancer selects a random healthy host. The random
	// load balancer generally performs better than round robin if no health
	// checking policy is configured.
	SimpleLBRandom SimpleLB = "RANDOM"

	// This option will forward the connection to the original IP address
	// requested by the caller without doing any form of load
	// balancing. This option must be used with care. It is meant for
	// advanced use cases. Refer to Original Destination load balancer in
	// Envoy for further details.
	SimpleLBPassthrough SimpleLB = "PASSTHROUGH"

	// The least request load balancer spreads load across endpoints,
	// favoring endpoints with the least outstanding requests. This is generally
	// safer and outperforms ROUND_ROBIN in nearly all cases. Prefer to use LEAST_REQUEST
	// as a drop-in replacement for ROUND_ROBIN.
	SimpleLBLeastRequest SimpleLB = "LEAST_REQUEST"
)

// Consistent Hash-based load balancing can be used to provide soft
// session affinity based on HTTP headers, cookies or other
// properties. This load balancing policy is applicable only for HTTP
// connections. The affinity to a particular destination host will be
// lost when one or more hosts are added/removed from the destination
// service.
type ConsistentHashLB struct {

	// It is required to specify exactly one of the fields as hash key:
	// HTTPHeaderName, HTTPCookie, or UseSourceIP.
	// Hash based on a specific HTTP header.
	HTTPHeaderName string `json:"httpHeaderName,omitempty"`

	// Hash based on HTTP cookie.
	HTTPCookie *HTTPCookie `json:"httpCookie,omitempty"`

	// Hash based on the source IP address.
	UseSourceIP bool `json:"useSourceIp,omitempty"`

	// The minimum number of virtual nodes to use for the hash
	// ring. Defaults to 1024. Larger ring sizes result in more granular
	// load distributions. If the number of hosts in the load balancing
	// pool is larger than the ring size, each host will be assigned a
	// single virtual node.
	MinimumRingSize uint64 `json:"minimumRingSize,omitempty"`
}

// Describes a HTTP cookie that will be used as the hash key for the
// Consistent Hash load balancer. If the cookie is not present, it will
// be generated.
type HTTPCookie struct {
	// REQUIRED. Name of the cookie.
	Name string `json:"name"`

	// Path to set for the cookie.
	Path string `json:"path,omitempty"`

	// REQUIRED. Lifetime of the cookie.
	TTL string `json:"ttl"`
}

// Connection pool settings for an upstream host. The settings apply to
// each individual host in the upstream service.  See Envoy's [circuit
// breaker](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/circuit_breaking)
// for more details. Connection pool settings can be applied at the TCP
// level as well as at HTTP level.
//
// For example, the following rule sets a limit of 100 connections to redis
// service called myredissrv with a connect timeout of 30ms
//
// apiVersion: networking.istio.io/v1alpha3
// kind: DestinationRule
// metadata:
//
//	name: bookinfo-redis
//
// spec:
//
//	host: myredissrv.prod.svc.cluster.local
//	trafficPolicy:
//	  connectionPool:
//	    tcp:
//	      maxConnections: 100
//	      connectTimeout: 30ms
type ConnectionPoolSettings struct {

	// Settings common to both HTTP and TCP upstream connections.
	TCP *TCPSettings `json:"tcp,omitempty"`

	// HTTP connection pool settings.
	HTTP *HTTPSettings `json:"http,omitempty"`
}

// Settings common to both HTTP and TCP upstream connections.
type TCPSettings struct {
	// Maximum number of HTTP1 /TCP connections to a destination host.
	MaxConnections int32 `json:"maxConnections,omitempty"`

	// TCP connection timeout.
	ConnectTimeout string `json:"connectTimeout,omitempty"`
}

// Settings applicable to HTTP1.1/HTTP2/GRPC connections.
type HTTPSettings struct {
	// Specify if http1.1 connection should be upgraded to http2 for the associated destination.
	// DEFAULT - Use the global default.
	// DO_NOT_UPGRADE - Do not upgrade the connection to http2.
	// UPGRADE - Upgrade the connection to http2.
	H2UpgradePolicy string `json:"h2UpgradePolicy,omitempty"`

	// Maximum number of pending HTTP requests to a destination. Default 2^32-1.
	HTTP1MaxPendingRequests int32 `json:"http1MaxPendingRequests,omitempty"`

	// Maximum number of requests to a backend. Default 2^32-1.
	HTTP2MaxRequests int32 `json:"http2MaxRequests,omitempty"`

	// Maximum number of requests per connection to a backend. Setting this
	// parameter to 1 disables keep alive. Default 0, meaning "unlimited",
	// up to 2^29.
	MaxRequestsPerConnection int32 `json:"maxRequestsPerConnection,omitempty"`

	// Maximum number of retries that can be outstanding to all hosts in a
	// cluster at a given time. Defaults to 2^32-1.
	MaxRetries int32 `json:"maxRetries,omitempty"`

	// The idle timeout for upstream connection pool connections. The idle timeout is defined as the period in which there are no active requests.
	// If not set, the default is 1 hour. When the idle timeout is reached the connection will be closed.
	// Note that request based timeouts mean that HTTP/2 PINGs will not keep the connection alive. Applies to both HTTP1.1 and HTTP2 connections.
	IdleTimeout string `json:"idleTimeout,omitempty"`
}

// A Circuit breaker implementation that tracks the status of each
// individual host in the upstream service.  Applicable to both HTTP and
// TCP services.  For HTTP services, hosts that continually return 5xx
// errors for API calls are ejected from the pool for a pre-defined period
// of time. For TCP services, connection timeouts or connection
// failures to a given host counts as an error when measuring the
// consecutive errors metric. See Envoy's [outlier
// detection](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/outlier)
// for more details.
//
// The following rule sets a connection pool size of 100 connections and
// 1000 concurrent HTTP2 requests, with no more than 10 req/connection to
// "reviews" service. In addition, it configures upstream hosts to be
// scanned every 5 mins, such that any host that fails 7 consecutive times
// with 5XX error code will be ejected for 15 minutes.
//
// apiVersion: networking.istio.io/v1alpha3
// kind: DestinationRule
// metadata:
//
//	name: reviews-cb-policy
//
// spec:
//
//	host: reviews.prod.svc.cluster.local
//	trafficPolicy:
//	  connectionPool:
//	    tcp:
//	      maxConnections: 100
//	    http:
//	      http2MaxRequests: 1000
//	      maxRequestsPerConnection: 10
//	  outlierDetection:
//	    consecutiveErrors: 7
//	    interval: 5m
//	    baseEjectionTime: 15m
type OutlierDetection struct {
	// Number of errors before a host is ejected from the connection
	// pool. Defaults to 5. When the upstream host is accessed over HTTP, a
	// 5xx return code qualifies as an error. When the upstream host is
	// accessed over an opaque TCP connection, connect timeouts and
	// connection error/failure events qualify as an error.
	ConsecutiveErrors int32 `json:"consecutiveErrors,omitempty"`

	// Number of gateway errors before a host is ejected from the connection pool.
	// When the upstream host is accessed over HTTP, a 502, 503, or 504 return
	// code qualifies as a gateway error. When the upstream host is accessed over
	// an opaque TCP connection, connect timeouts and connection error/failure
	// events qualify as a gateway error.
	// This feature is disabled by default or when set to the value 0.
	//
	// Note that consecutive_gateway_errors and consecutive_5xx_errors can be
	// used separately or together. Because the errors counted by
	// consecutive_gateway_errors are also included in consecutive_5xx_errors,
	// if the value of consecutive_gateway_errors is greater than or equal to
	// the value of consecutive_5xx_errors, consecutive_gateway_errors will have
	// no effect.
	ConsecutiveGatewayErrors *uint32 `json:"consecutiveGatewayErrors,omitempty"`

	// Number of 5xx errors before a host is ejected from the connection pool.
	// When the upstream host is accessed over an opaque TCP connection, connect
	// timeouts, connection error/failure and request failure events qualify as a
	// 5xx error.
	// This feature defaults to 5 but can be disabled by setting the value to 0.
	//
	// Note that consecutive_gateway_errors and consecutive_5xx_errors can be
	// used separately or together. Because the errors counted by
	// consecutive_gateway_errors are also included in consecutive_5xx_errors,
	// if the value of consecutive_gateway_errors is greater than or equal to
	// the value of consecutive_5xx_errors, consecutive_gateway_errors will have
	// no effect.
	Consecutive5xxErrors *uint32 `json:"consecutive5xxErrors,omitempty"`

	// Time interval between ejection sweep analysis. format:
	// 1h/1m/1s/1ms. MUST BE >=1ms. Default is 10s.
	Interval string `json:"interval,omitempty"`

	// Minimum ejection duration. A host will remain ejected for a period
	// equal to the product of minimum ejection duration and the number of
	// times the host has been ejected. This technique allows the system to
	// automatically increase the ejection period for unhealthy upstream
	// servers. format: 1h/1m/1s/1ms. MUST BE >=1ms. Default is 30s.
	BaseEjectionTime string `json:"baseEjectionTime,omitempty"`

	// Maximum % of hosts in the load balancing pool for the upstream
	// service that can be ejected. Defaults to 10%.
	MaxEjectionPercent int32 `json:"maxEjectionPercent,omitempty"`

	// Outlier detection will be enabled as long as the associated load balancing
	// pool has at least min_health_percent hosts in healthy mode. When the
	// percentage of healthy hosts in the load balancing pool drops below this
	// threshold, outlier detection will be disabled and the proxy will load balance
	// across all hosts in the pool (healthy and unhealthy). The threshold can be
	// disabled by setting it to 0%. The default is 0% as it's not typically
	// applicable in k8s environments with few pods per service.
	MinHealthPercent int32 `json:"minHealthPercent,omitempty"`
}

// SSL/TLS related settings for upstream connections. See Envoy's [TLS
// context](https://www.envoyproxy.io/docs/envoy/latest/api-v1/cluster_manager/cluster_ssl.html#config-cluster-manager-cluster-ssl)
// for more details. These settings are common to both HTTP and TCP upstreams.
//
// For example, the following rule configures a client to use mutual TLS
// for connections to upstream database cluster.
//
// apiVersion: networking.istio.io/v1alpha3
// kind: DestinationRule
// metadata:
//
//	name: db-mtls
//
// spec:
//
//	host: mydbserver.prod.svc.cluster.local
//	trafficPolicy:
//	  tls:
//	    mode: MUTUAL
//	    clientCertificate: /etc/certs/myclientcert.pem
//	    privateKey: /etc/certs/client_private_key.pem
//	    caCertificates: /etc/certs/rootcacerts.pem
//
// The following rule configures a client to use TLS when talking to a
// foreign service whose domain matches *.foo.com.
//
// apiVersion: networking.istio.io/v1alpha3
// kind: DestinationRule
// metadata:
//
//	name: tls-foo
//
// spec:
//
//	host: "*.foo.com"
//	trafficPolicy:
//	  tls:
//	    mode: SIMPLE
//
// The following rule configures a client to use Istio mutual TLS when talking
// to rating services.
//
// apiVersion: networking.istio.io/v1alpha3
// kind: DestinationRule
// metadata:
//
//	name: ratings-istio-mtls
//
// spec:
//
//	host: ratings.prod.svc.cluster.local
//	trafficPolicy:
//	  tls:
//	    mode: ISTIO_MUTUAL
type TLSSettings struct {

	// REQUIRED: Indicates whether connections to this port should be secured
	// using TLS. The value of this field determines how TLS is enforced.
	Mode TLSmode `json:"mode"`

	// REQUIRED if mode is `MUTUAL`. The path to the file holding the
	// client-side TLS certificate to use.
	// Should be empty if mode is `ISTIO_MUTUAL`.
	ClientCertificate string `json:"clientCertificate,omitempty"`

	// REQUIRED if mode is `MUTUAL`. The path to the file holding the
	// client's private key.
	// Should be empty if mode is `ISTIO_MUTUAL`.
	PrivateKey string `json:"privateKey,omitempty"`

	// OPTIONAL: The path to the file containing certificate authority
	// certificates to use in verifying a presented server certificate. If
	// omitted, the proxy will not verify the server's certificate.
	// Should be empty if mode is `ISTIO_MUTUAL`.
	CaCertificates string `json:"caCertificates,omitempty"`

	// A list of alternate names to verify the subject identity in the
	// certificate. If specified, the proxy will verify that the server
	// certificate's subject alt name matches one of the specified values.
	// Should be empty if mode is `ISTIO_MUTUAL`.
	SubjectAltNames []string `json:"subjectAltNames,omitempty"`

	// SNI string to present to the server during TLS handshake.
	// Should be empty if mode is `ISTIO_MUTUAL`.
	Sni string `json:"sni,omitempty"`
}

// TLS connection mode
type TLSmode string

const (
	// Do not setup a TLS connection to the upstream endpoint.
	TLSmodeDisable TLSmode = "DISABLE"

	// Originate a TLS connection to the upstream endpoint.
	TLSmodeSimple TLSmode = "SIMPLE"

	// Secure connections to the upstream using mutual TLS by presenting
	// client certificates for authentication.
	TLSmodeMutual TLSmode = "MUTUAL"

	// Secure connections to the upstream using mutual TLS by presenting
	// client certificates for authentication.
	// Compared to Mutual mode, this mode uses certificates generated
	// automatically by Istio for mTLS authentication. When this mode is
	// used, all other fields in `TLSSettings` should be empty.
	TLSmodeIstioMutual TLSmode = "ISTIO_MUTUAL"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// DestinationRuleList is a list of DestinationRule resources
type DestinationRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []DestinationRule `json:"items"`
}
