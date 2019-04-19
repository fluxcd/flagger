package kubepod

import (
	"reflect"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	kubev1 "k8s.io/api/core/v1"
)

type Pod kubev1.Pod

func (p *Pod) Clone() *Pod {
	vp := kubev1.Pod(*p)
	copy := vp.DeepCopy()
	newP := Pod(*copy)
	return &newP
}

func (p *Pod) GetMetadata() core.Metadata {
	return kubeutils.FromKubeMeta(p.ObjectMeta)
}

func (p *Pod) SetMetadata(meta core.Metadata) {
	p.ObjectMeta = kubeutils.ToKubeMeta(meta)
}

func (p *Pod) Equal(that interface{}) bool {
	return reflect.DeepEqual(p, that)
}
