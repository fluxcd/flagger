module github.com/weaveworks/flagger

go 1.13

require (
	cloud.google.com/go v0.37.4 // indirect
	github.com/Masterminds/semver v1.4.2
	github.com/beorn7/perks v1.0.0 // indirect
	github.com/gogo/protobuf v1.2.1 // indirect
	github.com/golang/groupcache v0.0.0-20190129154638-5b532d6fd5ef // indirect
	github.com/golang/protobuf v1.3.1 // indirect
	github.com/google/go-cmp v0.3.0
	github.com/googleapis/gnostic v0.2.0 // indirect
	github.com/hashicorp/golang-lru v0.5.1 // indirect
	github.com/imdario/mergo v0.3.7 // indirect
	github.com/kr/pretty v0.1.0 // indirect
	github.com/mitchellh/hashstructure v1.0.0
	github.com/pkg/errors v0.8.1
	github.com/prometheus/client_golang v0.9.3-0.20190127221311-3c4408c8b829
	github.com/prometheus/client_model v0.0.0-20190129233127-fd36f4220a90 // indirect
	github.com/prometheus/common v0.3.0 // indirect
	github.com/prometheus/procfs v0.0.0-20190416084830-8368d24ba045 // indirect
	go.uber.org/atomic v1.3.2 // indirect
	go.uber.org/multierr v1.1.0 // indirect
	go.uber.org/zap v1.9.1
	golang.org/x/crypto v0.0.0-20190418161225-b43e412143f9 // indirect
	golang.org/x/net v0.0.0-20190503192946-f4e77d36d62c // indirect
	golang.org/x/sys v0.0.0-20190508220229-2d0786266e9c // indirect
	golang.org/x/text v0.3.2 // indirect
	golang.org/x/time v0.0.0-20190308202827-9d24e82272b4 // indirect
	gopkg.in/check.v1 v1.0.0-20180628173108-788fd7840127 // indirect
	gopkg.in/h2non/gock.v1 v1.0.14
	gopkg.in/inf.v0 v0.9.1 // indirect
	k8s.io/api v0.0.0-20190620073856-dcce3486da33
	k8s.io/apimachinery v0.0.0-20190620073744-d16981aedf33
	k8s.io/client-go v11.0.0+incompatible
	k8s.io/code-generator v0.0.0-20190620073620-d55040311883
	k8s.io/kube-openapi v0.0.0-20190418160015-6b3d3b2d5666 // indirect
	k8s.io/utils v0.0.0-20190308190857-21c4ce38f2a7 // indirect
)

replace (
	github.com/google/uuid => github.com/google/uuid v1.0.0
	golang.org/x/crypto => golang.org/x/crypto v0.0.0-20181025213731-e84da0312774
	golang.org/x/net => golang.org/x/net v0.0.0-20190206173232-65e2d4e15006
	golang.org/x/sync => golang.org/x/sync v0.0.0-20181108010431-42b317875d0f
	golang.org/x/sys => golang.org/x/sys v0.0.0-20190209173611-3b5209105503
	golang.org/x/tools => golang.org/x/tools v0.0.0-20190313210603-aa82965741a9
	k8s.io/api => k8s.io/api v0.0.0-20190620073856-dcce3486da33
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20190620073744-d16981aedf33
	k8s.io/client-go => k8s.io/client-go v0.0.0-20190620074045-585a16d2e773
	k8s.io/code-generator => k8s.io/code-generator v0.0.0-20190620073620-d55040311883
	k8s.io/component-base => k8s.io/component-base v0.0.0-20190620074451-e5083e713460
)

replace k8s.io/klog => github.com/stefanprodan/klog v0.0.0-20190418165334-9cbb78b20423
