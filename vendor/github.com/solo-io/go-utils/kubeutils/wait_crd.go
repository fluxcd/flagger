package kubeutils

import (
	"time"

	"github.com/avast/retry-go"
	"github.com/solo-io/go-utils/errors"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiexts "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Waits for a CRD to be "established" in kubernetes, which means it's active an can be
// CRUD'ed by clients
func WaitForCrdActive(apiexts apiexts.Interface, crdName string) error {
	return retry.Do(func() error {
		crd, err := apiexts.ApiextensionsV1beta1().CustomResourceDefinitions().Get(crdName, metav1.GetOptions{})
		if err != nil {
			return errors.Wrapf(err, "lookup crd %v", crdName)
		}

		var established bool
		for _, status := range crd.Status.Conditions {
			if status.Type == v1beta1.Established {
				established = true
				break
			}
		}

		if !established {
			return errors.Errorf("crd %v exists but not yet established by kube", crdName)
		}

		return nil
	},
		retry.Delay(time.Millisecond*500),
		retry.DelayType(retry.FixedDelay),
	)
}
