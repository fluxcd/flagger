module github.com/fluxcd/flagger

go 1.15

require (
	github.com/Masterminds/semver/v3 v3.0.3
	github.com/aws/aws-sdk-go v1.37.32
	github.com/davecgh/go-spew v1.1.1
	github.com/docker/spdystream v0.0.0-20160310174837-449fdfce4d96 // indirect
	github.com/go-kit/kit v0.10.0 // indirect
	github.com/go-logr/zapr v0.4.0
	github.com/go-openapi/spec v0.19.3 // indirect
	github.com/google/go-cmp v0.5.5
	github.com/prometheus/client_golang v1.11.0
	github.com/stretchr/testify v1.7.0
	go.uber.org/zap v1.19.0
	gopkg.in/h2non/gock.v1 v1.0.15
	k8s.io/api v0.22.1
	k8s.io/apimachinery v0.22.1
	k8s.io/client-go v0.22.1
	k8s.io/code-generator v0.22.1
	k8s.io/klog/v2 v2.9.0
	sigs.k8s.io/controller-runtime v0.10.0
)
