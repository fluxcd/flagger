package operator

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	flaggerv1 "github.com/stefanprodan/flagger/pkg/apis/flagger/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *Controller) saveDeploymentSpec(cd *flaggerv1.CanaryDeployment, dep *appsv1.Deployment) error {
	specJson, err := json.Marshal(dep.Spec.Template.Spec)
	if err != nil {
		return err
	}

	specEnc := base64.StdEncoding.EncodeToString(specJson)
	cd.Status.CanaryRevision = specEnc
	cd, err = c.rolloutClient.FlaggerV1beta1().CanaryDeployments(cd.Namespace).Update(cd)
	if err != nil {
		return err
	}
	return nil
}

func (c *Controller) diffDeploymentSpec(cd *flaggerv1.CanaryDeployment, dep *appsv1.Deployment) (bool, error) {
	if cd.Status.CanaryRevision == "" {
		return true, nil
	}

	newSpec := &dep.Spec.Template.Spec
	oldSpecJson, err := base64.StdEncoding.DecodeString(cd.Status.CanaryRevision)
	if err != nil {
		return false, err
	}
	oldSpec := &corev1.PodSpec{}
	err = json.Unmarshal(oldSpecJson, oldSpec)
	if err != nil {
		return false, err
	}

	if diff := cmp.Diff(*newSpec, *oldSpec, cmpopts.IgnoreUnexported(resource.Quantity{})); diff != "" {
		fmt.Println(diff)
		return true, nil
	}

	return false, nil
}

func (c *Controller) getDeploymentSpec(name string, namespace string) (string, error) {
	dep, err := c.kubeClient.AppsV1().Deployments(namespace).Get(name, v1.GetOptions{})
	if err != nil {
		return "", err
	}

	specJson, err := json.Marshal(dep.Spec.Template.Spec)
	if err != nil {
		return "", err
	}

	specEnc := base64.StdEncoding.EncodeToString(specJson)
	return specEnc, nil
}

func (c *Controller) getDeploymentSpecEnc(dep *appsv1.Deployment) (string, error) {
	specJson, err := json.Marshal(dep.Spec.Template.Spec)
	if err != nil {
		return "", err
	}

	specEnc := base64.StdEncoding.EncodeToString(specJson)
	return specEnc, nil
}
