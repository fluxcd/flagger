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
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	clientset "github.com/fluxcd/flagger/pkg/client/clientset/versioned"
)

// specHashPrefix marks hashes computed from the canonical JSON representation
// of the target spec. Hashes without the prefix were computed by previous
// Flagger versions with spew over the decoded Go structs; those encode the
// k8s.io/api struct schema and are not comparable across binaries, so they
// are migrated by migrateSpecTracking instead of being compared.
const specHashPrefix = "v2:"

// targetSnapshot captures the state of a canary target used for change
// detection. Both fields are taken from the same object read, so that
// (status.lastAppliedSpec, status.lastTrackedRevision) always forms a
// consistent pair: the hash describes the object at exactly that revision.
type targetSnapshot struct {
	// hash is the content hash of the normalized target spec.
	hash string
	// fence is <uid>/<counter>[/<revision>], where counter is a
	// server-maintained change indicator: metadata.generation for
	// Deployments and DaemonSets, metadata.resourceVersion for Services and
	// the latest created revision name for Knative Services. The optional
	// third component is the deployment.kubernetes.io/revision annotation,
	// recorded only while it provably corresponds to the hashed template
	// (deployment not paused and observedGeneration caught up).
	fence string
	// volatileCounter marks counters that move on writes unrelated to the
	// spec (resourceVersion changes on status updates and metadata churn);
	// chasing such a counter would cost one status write per reconcile, so
	// the fence is only refreshed by the regular status syncs.
	volatileCounter bool
}

// hasSpecChanged compares the target snapshot against the tracking fields in
// the canary status and decides whether a new revision must be analyzed.
// The fence tells apart real spec writes from hash drift: when the fence
// proves the spec did not change, a differing hash is an artifact of a
// Flagger or cluster upgrade and is absorbed by re-baselining the status.
// Absorb, fence-refresh and migration decisions are persisted through the
// status subresource without mutating cd in place; a failed write only delays
// persistence until the next reconcile, it never changes the verdict.
func hasSpecChanged(logger *zap.SugaredLogger, flaggerClient clientset.Interface,
	cd *flaggerv1.Canary, snap targetSnapshot, imagesDiffer func() (bool, error)) (bool, error) {
	logger = logger.With("canary", fmt.Sprintf("%s.%s", cd.Name, cd.Namespace))

	applied := cd.Status.LastAppliedSpec
	if applied == "" {
		return true, nil
	}

	// status written by a previous Flagger version
	if !strings.HasPrefix(applied, specHashPrefix) || cd.Status.LastTrackedRevision == "" {
		return migrateSpecTracking(logger, flaggerClient, cd, snap, imagesDiffer)
	}

	storedUID, storedCounter, storedRevision := parseFence(cd.Status.LastTrackedRevision)
	currentUID, currentCounter, currentRevision := parseFence(snap.fence)

	// a deleted and recreated target is always a new revision
	if storedUID != currentUID {
		return true, nil
	}

	// the fence is only trustworthy when the server actually maintains the
	// counter (generation is 0 when unset, e.g. on fake clients); an
	// untrusted fence must behave as "changed counter" so decisions fall
	// back to hash comparison and never absorb
	fenceCapable := currentUID != "" && currentCounter != "" && currentCounter != "0"

	sameCounter := fenceCapable && storedCounter == currentCounter

	if snap.hash == applied {
		// content already tracked; keep the whole fence current (a
		// replicas-only change moves the counter, and the deployment
		// revision component may become available once the deployment
		// controller catches up) so future hash drift can be absorbed
		if !snap.volatileCounter && cd.Status.LastTrackedRevision != snap.fence {
			persistOrLog(logger, flaggerClient, cd, snap, "fence refreshed")
		}
		return false, nil
	}

	// the hash differs from the last applied spec: absorb the difference iff
	// the fence proves that no spec change happened, which makes the
	// difference an artifact of a hash algorithm or schema change
	specUnchanged := sameCounter ||
		(fenceCapable && storedRevision != "" && currentRevision != "" && storedRevision == currentRevision)
	if specUnchanged {
		persistOrLog(logger, flaggerClient, cd, snap, "hash drift absorbed")
		return false, nil
	}

	// do not trigger a canary deployment on manual rollback
	if snap.hash == cd.Status.LastPromotedSpec {
		persistOrLog(logger, flaggerClient, cd, snap, "manual rollback detected")
		return false, nil
	}

	return true, nil
}

// migrateSpecTracking handles canaries whose status was written by a previous
// Flagger version (spew hash, no fence). Legacy hashes cannot be compared
// with the current ones, so the decision is made by phase: settled canaries
// adopt the current target state as their baseline without triggering, while
// canaries with an analysis in flight or in a failed state are reported as
// changed, restarting the analysis once after the upgrade — loud but safe,
// since adopting mid-run could silently swallow a change made while Flagger
// was down. Waiting, Promoting and Finalising canaries never reach this code:
// the scheduler advances them without checking for target changes, and their
// next status sync records the new tracking format.
func migrateSpecTracking(logger *zap.SugaredLogger, flaggerClient clientset.Interface,
	cd *flaggerv1.Canary, snap targetSnapshot, imagesDiffer func() (bool, error)) (bool, error) {
	switch cd.Status.Phase {
	case flaggerv1.CanaryPhaseInitialized, flaggerv1.CanaryPhaseSucceeded:
		if imagesDiffer != nil {
			differ, err := imagesDiffer()
			if err != nil {
				// neither adopt (could swallow a change applied while
				// Flagger was down) nor trigger (could start a spurious
				// rollout); the reconciler retries on the next sync
				return false, fmt.Errorf("spec tracking migration blocked, image comparison failed: %w", err)
			}
			if differ {
				// the canary images differ from the primary ones, a rollout
				// was most likely applied while Flagger was not running
				return true, nil
			}
		}
		persistOrLog(logger, flaggerClient, cd, snap, "spec tracking migrated")
		return false, nil
	default:
		return true, nil
	}
}

// persistOrLog persists the (hash, fence) pair with persistSpecTracking and
// logs the outcome; persistence failures are not fatal, the decision is
// simply re-made and re-persisted on the next reconcile.
func persistOrLog(logger *zap.SugaredLogger, flaggerClient clientset.Interface,
	cd *flaggerv1.Canary, snap targetSnapshot, reason string) {
	if err := persistSpecTracking(flaggerClient, cd, snap); err != nil {
		logger.Warnf("failed to persist spec tracking (%s), retrying next reconcile: %v", reason, err)
		return
	}
	logger.Infof("spec tracking updated: %s", reason)
}

// encodeFence builds the lastTrackedRevision value from the target's uid, a
// server-maintained change counter and an optional deployment revision.
func encodeFence(uid types.UID, counter, revision string) string {
	if revision != "" {
		return fmt.Sprintf("%s/%s/%s", uid, counter, revision)
	}
	return fmt.Sprintf("%s/%s", uid, counter)
}

// parseFence splits a lastTrackedRevision value into its components; missing
// components are returned as empty strings, which never compare as matching.
func parseFence(fence string) (uid, counter, revision string) {
	parts := strings.SplitN(fence, "/", 3)
	if len(parts) > 0 {
		uid = parts[0]
	}
	if len(parts) > 1 {
		counter = parts[1]
	}
	if len(parts) > 2 {
		revision = parts[2]
	}
	return
}

// ComputeSpecHash returns the content hash of a Kubernetes object spec,
// computed from its canonical JSON representation: unset fields are omitted
// and map keys are sorted, so the hash does not change when the k8s.io/api
// structs gain new fields on a Flagger dependency upgrade — only when the
// object content changes.
func ComputeSpecHash(spec interface{}) (string, error) {
	data, err := json.Marshal(spec)
	if err != nil {
		return "", fmt.Errorf("spec hash: marshal failed: %w", err)
	}

	// round-trip through an untyped value so that keys are re-serialized in
	// sorted order, independent of the Go struct field order; UseNumber
	// preserves the exact numeric representation
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var untyped interface{}
	if err := decoder.Decode(&untyped); err != nil {
		return "", fmt.Errorf("spec hash: decode failed: %w", err)
	}
	canonical, err := json.Marshal(untyped)
	if err != nil {
		return "", fmt.Errorf("spec hash: canonical marshal failed: %w", err)
	}

	return hashBytes(canonical), nil
}

// ComputeStringHash returns the content hash of a plain string identifier
// (e.g. a Knative revision name); strings must not go through the JSON
// canonicalization used for object specs.
func ComputeStringHash(s string) string {
	return hashBytes([]byte(s))
}

func hashBytes(b []byte) string {
	digest := sha256.Sum256(b)
	return specHashPrefix + hex.EncodeToString(digest[:8])
}

// podImagesDiffer compares the init and regular container images of two pod
// specs by container name; used as a safety heuristic during spec tracking
// migration to catch rollouts applied while Flagger was not running.
func podImagesDiffer(a, b corev1.PodSpec) bool {
	am, bm := podImages(a), podImages(b)
	if len(am) != len(bm) {
		return true
	}
	for name, image := range am {
		if bm[name] != image {
			return true
		}
	}
	return false
}

func podImages(spec corev1.PodSpec) map[string]string {
	images := make(map[string]string, len(spec.Containers)+len(spec.InitContainers))
	for _, c := range spec.InitContainers {
		images["init:"+c.Name] = c.Image
	}
	for _, c := range spec.Containers {
		images[c.Name] = c.Image
	}
	return images
}

// ComputeHash returns a hash value calculated from a spec using the spew library
// which follows pointers and prints actual values of the nested objects
// ensuring the hash does not change when a pointer changes.
// Deprecated for spec change detection (the output depends on the k8s.io/api
// struct schema, see ComputeSpecHash); still used for event payload UUIDs.
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
