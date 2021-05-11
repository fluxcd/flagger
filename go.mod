module github.com/fluxcd/flagger

go 1.15

require (
	github.com/Masterminds/semver/v3 v3.1.0
	github.com/aws/aws-sdk-go v1.37.32
	github.com/davecgh/go-spew v1.1.1
	github.com/go-logr/zapr v0.3.0
	github.com/google/go-cmp v0.5.2
	github.com/prometheus/client_golang v1.9.0
	// gloo-v1.8.0-beta7
	github.com/solo-io/solo-apis v0.0.0-20210415143354-b4af9c42facf
	github.com/stretchr/testify v1.7.0
	go.uber.org/zap v1.15.0
	golang.org/x/tools v0.1.0 // indirect
	gopkg.in/h2non/gock.v1 v1.0.15
	k8s.io/api v0.20.4
	k8s.io/apimachinery v0.20.4
	k8s.io/client-go v8.0.0+incompatible
	k8s.io/code-generator v0.20.4
	k8s.io/klog/v2 v2.4.0
)

replace (
	k8s.io/client-go => k8s.io/client-go v0.20.4
	k8s.io/code-generator => k8s.io/code-generator v0.20.4
)
