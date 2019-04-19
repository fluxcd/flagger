package linkerd

import (
	"reflect"

	"github.com/linkerd/linkerd2/controller/gen/apis/serviceprofile/v1alpha1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
)

type ServiceProfile v1alpha1.ServiceProfile

func (p *ServiceProfile) GetMetadata() core.Metadata {
	return kubeutils.FromKubeMeta(p.ObjectMeta)
}

func (p *ServiceProfile) SetMetadata(meta core.Metadata) {
	p.ObjectMeta = kubeutils.ToKubeMeta(meta)
}

func (p *ServiceProfile) Equal(that interface{}) bool {
	return reflect.DeepEqual(p, that)
}

func (p *ServiceProfile) Clone() *ServiceProfile {
	vp := v1alpha1.ServiceProfile(*p)
	copy := vp.DeepCopy()
	newP := ServiceProfile(*copy)
	return &newP
}
