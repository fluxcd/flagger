module github.com/fluxcd/flagger

go 1.15

require (
	github.com/Masterminds/semver/v3 v3.0.3
	github.com/aws/aws-sdk-go v1.37.32
	github.com/davecgh/go-spew v1.1.1
	github.com/go-logr/zapr v0.3.0
	github.com/google/go-cmp v0.5.2
	github.com/prometheus/client_golang v1.9.0
	github.com/stretchr/testify v1.7.0
	go.uber.org/zap v1.14.1
	golang.org/x/tools v0.1.0 // indirect
	gopkg.in/h2non/gock.v1 v1.0.15
	k8s.io/api v0.20.4
	k8s.io/apimachinery v0.20.4
	k8s.io/client-go v0.20.4
	k8s.io/code-generator v0.20.4
	k8s.io/klog/v2 v2.4.0
)
