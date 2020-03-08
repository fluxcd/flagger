module github.com/weaveworks/flagger

go 1.14

require (
	github.com/Masterminds/semver/v3 v3.0.3
	github.com/aws/aws-sdk-go v1.29.12
	github.com/davecgh/go-spew v1.1.1
	github.com/golang/groupcache v0.0.0-20190129154638-5b532d6fd5ef // indirect
	github.com/google/go-cmp v0.3.0
	github.com/googleapis/gnostic v0.2.0 // indirect
	github.com/hashicorp/consul/api v1.4.0
	github.com/imdario/mergo v0.3.7 // indirect
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.0.0
	github.com/prometheus/common v0.4.1
	github.com/stretchr/testify v1.5.1
	go.uber.org/atomic v1.3.2 // indirect
	go.uber.org/multierr v1.1.0 // indirect
	go.uber.org/zap v1.10.0
	gopkg.in/h2non/gock.v1 v1.0.14
	k8s.io/api v0.17.2
	k8s.io/apimachinery v0.17.2
	k8s.io/client-go v0.17.2
	k8s.io/code-generator v0.17.2
	k8s.io/utils v0.0.0-20191114184206-e782cd3c129f
)

replace k8s.io/klog => github.com/stefanprodan/klog v0.0.0-20190418165334-9cbb78b20423
