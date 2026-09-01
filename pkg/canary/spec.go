/*
Copyright 2020 The Flux authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package canary

import (
	"context"
	"fmt"
	"hash/fnv"

	"github.com/davecgh/go-spew/spew"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/kubernetes"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

// hasSpecChanged computes the hash of the spec and compares it with the
// last applied spec, if the last applied hash is different but not equal
// to last promoted one the it returns true
func hasSpecChanged(cd *flaggerv1.Canary, spec interface{}) (bool, error) {
	if cd.Status.LastAppliedSpec == "" {
		return true, nil
	}

	newHash := ComputeHash(spec)

	// do not trigger a canary deployment on manual rollback
	if cd.Status.LastPromotedSpec == newHash {
		return false, nil
	}

	if cd.Status.LastAppliedSpec != newHash {
		return true, nil
	}

	return false, nil
}

// hasDeploymentSpecChanged checks if the deployment's  server-side generated
// pod-template-hash has changed by comparing with the last applied spec. If
// the last applied hash is different but not equal to last promoted one then
// it returns true
func hasDeploymentSpecChanged(kubeClient kubernetes.Interface, cd *flaggerv1.Canary, deployment *appsv1.Deployment) (bool, error) {
	if cd.Status.LastAppliedSpec == "" {
		return true, nil
	}

	// Get the current server-side hash
	newHash, err := GetReplicaSetHash(kubeClient, deployment)
	if err != nil {
		return false, fmt.Errorf("failed to get current server-side hash: %w", err)
	}

	// Do not trigger a canary deployment on manual rollback
	if cd.Status.LastPromotedSpec == newHash {
		return false, nil
	}

	if cd.Status.LastAppliedSpec != newHash {
		return true, nil
	}

	return false, nil
}

// ComputeHash returns a hash value calculated from a spec using the spew library
// which follows pointers and prints actual values of the nested objects
// ensuring the hash does not change when a pointer changes.
func ComputeHash(spec interface{}) string {
	hasher := fnv.New32a()
	printer := spew.ConfigState{
		Indent:         " ",
		SortKeys:       true,
		DisableMethods: true,
		SpewKeys:       true,
	}
	printer.Fprintf(hasher, "%#v", spec)

	return rand.SafeEncodeString(fmt.Sprint(hasher.Sum32()))
}

// GetReplicaSetHash returns the pod-template-hash of the current ReplicaSet of
// the Deployment, by matching the "deployment.kubernetes.io/revision"
// annotation. As pod-template-hash is server-generated, it remains consistent
// across Kubernetes client library versions.
func GetReplicaSetHash(kubeClient kubernetes.Interface, deployment *appsv1.Deployment) (string, error) {
	revisionAnnotation := "deployment.kubernetes.io/revision"
	revision, exists := deployment.Annotations[revisionAnnotation]
	if !exists {
		return "", fmt.Errorf("missing revision annotation for %s.%s", deployment.Name, deployment.Namespace)
	}

	// Get all ReplicaSets for this deployment
	selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err != nil {
		return "", fmt.Errorf("invalid label selector for %s.%s: %w", deployment.Name, deployment.Namespace, err)
	}

	replicaSets, err := kubeClient.AppsV1().ReplicaSets(deployment.Namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return "", fmt.Errorf("failed to list ReplicaSets for %s.%s: %w", deployment.Name, deployment.Namespace, err)
	}

	// Find the ReplicaSet with the matching "deployment.kubernetes.io/revision"
	// annotation as the Deployment. If found, return its "pod-template-hash"
	// label.
	for _, rs := range replicaSets.Items {
		rev, exists := rs.Annotations[revisionAnnotation]
		if !exists {
			continue
		}
		if rev != revision {
			continue
		}
		hash, exists := rs.Labels["pod-template-hash"]
		if !exists {
			continue
		}
		return hash, nil
	}

	return "", fmt.Errorf("failed to find pod-template-hash from a matching ReplicaSet for %s.%s", deployment.Name, deployment.Namespace)
}
