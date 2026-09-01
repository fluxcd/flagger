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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stesting "k8s.io/client-go/testing"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	clientset "github.com/fluxcd/flagger/pkg/client/clientset/versioned"
	fakeFlagger "github.com/fluxcd/flagger/pkg/client/clientset/versioned/fake"
)

func specTestFixture(status flaggerv1.CanaryStatus) (*flaggerv1.Canary, clientset.Interface, *zap.SugaredLogger) {
	cd := &flaggerv1.Canary{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "podinfo"},
		Spec: flaggerv1.CanarySpec{
			TargetRef: flaggerv1.LocalObjectReference{Name: "podinfo", Kind: "Deployment"},
		},
		Status: status,
	}
	return cd, fakeFlagger.NewSimpleClientset(cd), zap.NewNop().Sugar()
}

func getSpecTestCanary(t *testing.T, client clientset.Interface) *flaggerv1.Canary {
	t.Helper()
	fresh, err := client.FlaggerV1beta1().Canaries("default").Get(context.TODO(), "podinfo", metav1.GetOptions{})
	require.NoError(t, err)
	return fresh
}

func TestComputeSpecHash_Golden(t *testing.T) {
	// pinned expected hashes: if this test fails after a k8s.io/api upgrade,
	// the canonical JSON representation moved and unchanged workloads would
	// be re-hashed differently in production — investigate before releasing
	template := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "podinfo"}},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "podinfo", Image: "ghcr.io/stefanprodan/podinfo:6.0.0"},
			},
		},
	}
	hash, err := ComputeSpecHash(&template)
	require.NoError(t, err)
	assert.Equal(t, "v2:2941eab5be2776bc", hash)

	svcSpec := corev1.ServiceSpec{
		Selector: map[string]string{"app": "podinfo"},
		Ports:    []corev1.ServicePort{{Name: "http", Port: 9898}},
	}
	svcHash, err := ComputeSpecHash(&svcSpec)
	require.NoError(t, err)
	assert.Equal(t, "v2:a426cf88d6c4eb4a", svcHash)

	assert.Equal(t, "v2:ec91372f5617da08", ComputeStringHash("podinfo-00042"))
}

func TestComputeSpecHash_IgnoresUnsetFields(t *testing.T) {
	// a nil optional pointer field and an omitted one must hash identically:
	// this is the property that keeps hashes stable across k8s.io/api upgrades
	withNil := corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers:                    []corev1.Container{{Name: "app", Image: "img:1"}},
			TerminationGracePeriodSeconds: nil,
			NodeSelector:                  nil,
		},
	}
	withEmpty := corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers:   []corev1.Container{{Name: "app", Image: "img:1"}},
			NodeSelector: map[string]string{},
		},
	}
	a, err := ComputeSpecHash(&withNil)
	require.NoError(t, err)
	b, err := ComputeSpecHash(&withEmpty)
	require.NoError(t, err)
	assert.Equal(t, a, b)
}

func TestHasSpecChanged_FirstSync(t *testing.T) {
	cd, client, logger := specTestFixture(flaggerv1.CanaryStatus{})
	changed, err := hasSpecChanged(logger, client, cd, targetSnapshot{hash: "v2:aaaa", fence: "uid-1/5"}, nil)
	require.NoError(t, err)
	assert.True(t, changed)
}

func TestHasSpecChanged_UIDGate(t *testing.T) {
	cd, client, logger := specTestFixture(flaggerv1.CanaryStatus{
		Phase:               flaggerv1.CanaryPhaseSucceeded,
		LastAppliedSpec:     "v2:aaaa",
		LastPromotedSpec:    "v2:aaaa",
		LastTrackedRevision: "uid-1/5",
	})

	// same hash, but the target was deleted and recreated
	changed, err := hasSpecChanged(logger, client, cd, targetSnapshot{hash: "v2:aaaa", fence: "uid-2/1"}, nil)
	require.NoError(t, err)
	assert.True(t, changed)
}

func TestHasSpecChanged_AbsorbsDriftOnFenceMatch(t *testing.T) {
	cd, client, logger := specTestFixture(flaggerv1.CanaryStatus{
		Phase:               flaggerv1.CanaryPhaseSucceeded,
		LastAppliedSpec:     "v2:aaaa",
		LastPromotedSpec:    "v2:aaaa",
		LastTrackedRevision: "uid-1/5",
	})

	// hash drifted (simulated algorithm/schema change) while the fence
	// proves the spec did not change
	changed, err := hasSpecChanged(logger, client, cd, targetSnapshot{hash: "v2:bbbb", fence: "uid-1/5"}, nil)
	require.NoError(t, err)
	assert.False(t, changed)

	// the re-baseline is persisted with lastPromotedSpec in lockstep
	fresh := getSpecTestCanary(t, client)
	assert.Equal(t, "v2:bbbb", fresh.Status.LastAppliedSpec)
	assert.Equal(t, "v2:bbbb", fresh.Status.LastPromotedSpec)
	assert.Equal(t, "uid-1/5", fresh.Status.LastTrackedRevision)

	// the in-memory object is never mutated (Failed-phase re-sync asymmetry)
	assert.Equal(t, "v2:aaaa", cd.Status.LastAppliedSpec)
}

func TestHasSpecChanged_NoLockstepDuringRun(t *testing.T) {
	cd, client, logger := specTestFixture(flaggerv1.CanaryStatus{
		Phase:               flaggerv1.CanaryPhaseProgressing,
		LastAppliedSpec:     "v2:aaaa",
		LastPromotedSpec:    "v2:pppp",
		LastTrackedRevision: "uid-1/5",
	})

	changed, err := hasSpecChanged(logger, client, cd, targetSnapshot{hash: "v2:bbbb", fence: "uid-1/5"}, nil)
	require.NoError(t, err)
	assert.False(t, changed)

	// lastPromotedSpec pointed at another spec and must not move
	fresh := getSpecTestCanary(t, client)
	assert.Equal(t, "v2:bbbb", fresh.Status.LastAppliedSpec)
	assert.Equal(t, "v2:pppp", fresh.Status.LastPromotedSpec)
}

func TestHasSpecChanged_ReplicasOnlyChange(t *testing.T) {
	cd, client, logger := specTestFixture(flaggerv1.CanaryStatus{
		Phase:               flaggerv1.CanaryPhaseSucceeded,
		LastAppliedSpec:     "v2:aaaa",
		LastPromotedSpec:    "v2:aaaa",
		LastTrackedRevision: "uid-1/5",
	})

	// generation advanced (scale) but the template hash is unchanged
	changed, err := hasSpecChanged(logger, client, cd, targetSnapshot{hash: "v2:aaaa", fence: "uid-1/7"}, nil)
	require.NoError(t, err)
	assert.False(t, changed)

	fresh := getSpecTestCanary(t, client)
	assert.Equal(t, "uid-1/7", fresh.Status.LastTrackedRevision)
}

func TestHasSpecChanged_RevisionBackfill(t *testing.T) {
	// the fence was stored without a revision component (deployment
	// controller lagging at write time); once the revision becomes
	// trustworthy it must be persisted even though the counter is unchanged,
	// or the secondary absorb stays unavailable
	cd, client, logger := specTestFixture(flaggerv1.CanaryStatus{
		Phase:               flaggerv1.CanaryPhaseSucceeded,
		LastAppliedSpec:     "v2:aaaa",
		LastPromotedSpec:    "v2:aaaa",
		LastTrackedRevision: "uid-1/5",
	})

	changed, err := hasSpecChanged(logger, client, cd, targetSnapshot{hash: "v2:aaaa", fence: "uid-1/5/3"}, nil)
	require.NoError(t, err)
	assert.False(t, changed)

	fresh := getSpecTestCanary(t, client)
	assert.Equal(t, "uid-1/5/3", fresh.Status.LastTrackedRevision)
}

func TestHasSpecChanged_ManualRollback(t *testing.T) {
	cd, client, logger := specTestFixture(flaggerv1.CanaryStatus{
		Phase:               flaggerv1.CanaryPhaseSucceeded,
		LastAppliedSpec:     "v2:cccc",
		LastPromotedSpec:    "v2:aaaa",
		LastTrackedRevision: "uid-1/5",
	})

	// the target was rolled back to the promoted spec
	changed, err := hasSpecChanged(logger, client, cd, targetSnapshot{hash: "v2:aaaa", fence: "uid-1/9"}, nil)
	require.NoError(t, err)
	assert.False(t, changed)

	fresh := getSpecTestCanary(t, client)
	assert.Equal(t, "v2:aaaa", fresh.Status.LastAppliedSpec)
	assert.Equal(t, "v2:aaaa", fresh.Status.LastPromotedSpec)
	assert.Equal(t, "uid-1/9", fresh.Status.LastTrackedRevision)
}

func TestHasSpecChanged_RealChange(t *testing.T) {
	cd, client, logger := specTestFixture(flaggerv1.CanaryStatus{
		Phase:               flaggerv1.CanaryPhaseSucceeded,
		LastAppliedSpec:     "v2:aaaa",
		LastPromotedSpec:    "v2:aaaa",
		LastTrackedRevision: "uid-1/5",
	})

	changed, err := hasSpecChanged(logger, client, cd, targetSnapshot{hash: "v2:bbbb", fence: "uid-1/6"}, nil)
	require.NoError(t, err)
	assert.True(t, changed)

	// nothing is persisted on a real change; SyncStatus records it later
	fresh := getSpecTestCanary(t, client)
	assert.Equal(t, "v2:aaaa", fresh.Status.LastAppliedSpec)
	assert.Equal(t, "uid-1/5", fresh.Status.LastTrackedRevision)
}

func TestHasSpecChanged_SecondaryRevisionAbsorbs(t *testing.T) {
	cd, client, logger := specTestFixture(flaggerv1.CanaryStatus{
		Phase:               flaggerv1.CanaryPhaseSucceeded,
		LastAppliedSpec:     "v2:aaaa",
		LastPromotedSpec:    "v2:aaaa",
		LastTrackedRevision: "uid-1/5/3",
	})

	// generation advanced (e.g. HPA scale) AND the hash drifted, but the
	// deployment revision proves the template is unchanged
	changed, err := hasSpecChanged(logger, client, cd, targetSnapshot{hash: "v2:bbbb", fence: "uid-1/7/3"}, nil)
	require.NoError(t, err)
	assert.False(t, changed)

	fresh := getSpecTestCanary(t, client)
	assert.Equal(t, "v2:bbbb", fresh.Status.LastAppliedSpec)
}

func TestHasSpecChanged_SecondaryRevisionUnavailable(t *testing.T) {
	// when either side lacks a trusted revision (paused deployment or the
	// deployment controller lagging), scale+drift must trigger, not absorb
	for _, fences := range [][2]string{
		{"uid-1/5/3", "uid-1/7"},   // current revision untrusted
		{"uid-1/5", "uid-1/7/3"},   // stored revision untrusted
		{"uid-1/5/3", "uid-1/7/4"}, // revision moved: rollout happened
	} {
		cd, client, logger := specTestFixture(flaggerv1.CanaryStatus{
			Phase:               flaggerv1.CanaryPhaseSucceeded,
			LastAppliedSpec:     "v2:aaaa",
			LastPromotedSpec:    "v2:aaaa",
			LastTrackedRevision: fences[0],
		})
		changed, err := hasSpecChanged(logger, client, cd, targetSnapshot{hash: "v2:bbbb", fence: fences[1]}, nil)
		require.NoError(t, err)
		assert.True(t, changed, "stored %s current %s", fences[0], fences[1])
	}
}

func TestHasSpecChanged_IncapableFenceNeverAbsorbs(t *testing.T) {
	// generation 0 means the server does not maintain the counter (e.g. fake
	// clients); the fence must not prove anything
	cd, client, logger := specTestFixture(flaggerv1.CanaryStatus{
		Phase:               flaggerv1.CanaryPhaseSucceeded,
		LastAppliedSpec:     "v2:aaaa",
		LastPromotedSpec:    "v2:aaaa",
		LastTrackedRevision: "uid-1/0",
	})

	changed, err := hasSpecChanged(logger, client, cd, targetSnapshot{hash: "v2:bbbb", fence: "uid-1/0"}, nil)
	require.NoError(t, err)
	assert.True(t, changed)
}

func TestMigration_SettledPhaseAdopts(t *testing.T) {
	for _, phase := range []flaggerv1.CanaryPhase{flaggerv1.CanaryPhaseInitialized, flaggerv1.CanaryPhaseSucceeded} {
		cd, client, logger := specTestFixture(flaggerv1.CanaryStatus{
			Phase:            phase,
			LastAppliedSpec:  "6849403941", // legacy spew hash, no fence
			LastPromotedSpec: "6849403941",
		})

		changed, err := hasSpecChanged(logger, client, cd, targetSnapshot{hash: "v2:aaaa", fence: "uid-1/5"}, nil)
		require.NoError(t, err)
		assert.False(t, changed, string(phase))

		fresh := getSpecTestCanary(t, client)
		assert.Equal(t, "v2:aaaa", fresh.Status.LastAppliedSpec, string(phase))
		assert.Equal(t, "v2:aaaa", fresh.Status.LastPromotedSpec, string(phase))
		assert.Equal(t, "uid-1/5", fresh.Status.LastTrackedRevision, string(phase))

		// second evaluation with the migrated status takes the normal path
		// and stays silent: migration happens at most once per canary
		migrated := fresh.DeepCopy()
		changedAgain, err := hasSpecChanged(logger, client, migrated, targetSnapshot{hash: "v2:aaaa", fence: "uid-1/5"}, nil)
		require.NoError(t, err)
		assert.False(t, changedAgain, string(phase))
	}
}

func TestMigration_ImageDiffTriggers(t *testing.T) {
	cd, client, logger := specTestFixture(flaggerv1.CanaryStatus{
		Phase:            flaggerv1.CanaryPhaseSucceeded,
		LastAppliedSpec:  "6849403941",
		LastPromotedSpec: "6849403941",
	})

	// canary images differ from primary: a rollout landed while Flagger was
	// down; adopt would swallow it, so this must trigger
	changed, err := hasSpecChanged(logger, client, cd, targetSnapshot{hash: "v2:aaaa", fence: "uid-1/5"},
		func() (bool, error) { return true, nil })
	require.NoError(t, err)
	assert.True(t, changed)

	fresh := getSpecTestCanary(t, client)
	assert.Equal(t, "6849403941", fresh.Status.LastAppliedSpec)
}

func TestMigration_ImageDiffErrorBlocks(t *testing.T) {
	cd, client, logger := specTestFixture(flaggerv1.CanaryStatus{
		Phase:            flaggerv1.CanaryPhaseSucceeded,
		LastAppliedSpec:  "6849403941",
		LastPromotedSpec: "6849403941",
	})

	// on a failed comparison the migration must neither adopt nor trigger:
	// the error defers the decision to the next reconcile
	changed, err := hasSpecChanged(logger, client, cd, targetSnapshot{hash: "v2:aaaa", fence: "uid-1/5"},
		func() (bool, error) { return false, errors.New("primary not found") })
	require.Error(t, err)
	assert.False(t, changed)

	fresh := getSpecTestCanary(t, client)
	assert.Equal(t, "6849403941", fresh.Status.LastAppliedSpec)
	assert.Empty(t, fresh.Status.LastTrackedRevision)
}

func TestMigration_ActivePhasesTrigger(t *testing.T) {
	for _, phase := range []flaggerv1.CanaryPhase{flaggerv1.CanaryPhaseProgressing, flaggerv1.CanaryPhaseWaitingPromotion, flaggerv1.CanaryPhaseFailed} {
		cd, client, logger := specTestFixture(flaggerv1.CanaryStatus{
			Phase:           phase,
			LastAppliedSpec: "6849403941",
		})

		changed, err := hasSpecChanged(logger, client, cd, targetSnapshot{hash: "v2:aaaa", fence: "uid-1/5"}, nil)
		require.NoError(t, err)
		assert.True(t, changed, string(phase))
	}
}

func TestPersistSpecTracking_AbortsOnConcurrentWriter(t *testing.T) {
	cd, client, _ := specTestFixture(flaggerv1.CanaryStatus{
		Phase:               flaggerv1.CanaryPhaseSucceeded,
		LastAppliedSpec:     "v2:aaaa",
		LastTrackedRevision: "uid-1/5",
	})

	// another writer updated the tracked spec after the decision was made
	stale := cd.DeepCopy()
	stale.Status.LastAppliedSpec = "v2:old-decision-basis"

	require.NoError(t, persistSpecTracking(client, stale, targetSnapshot{hash: "v2:bbbb", fence: "uid-1/9"}))

	fresh := getSpecTestCanary(t, client)
	assert.Equal(t, "v2:aaaa", fresh.Status.LastAppliedSpec)
	assert.Equal(t, "uid-1/5", fresh.Status.LastTrackedRevision)
}

func TestHasSpecChanged_PersistFailureKeepsVerdict(t *testing.T) {
	cd, client, logger := specTestFixture(flaggerv1.CanaryStatus{
		Phase:               flaggerv1.CanaryPhaseSucceeded,
		LastAppliedSpec:     "v2:aaaa",
		LastPromotedSpec:    "v2:aaaa",
		LastTrackedRevision: "uid-1/5",
	})

	// all status writes fail: the absorb verdict must stand (no error, no
	// trigger) and the re-baseline is simply retried on the next reconcile
	fakeClient := client.(*fakeFlagger.Clientset)
	fakeClient.PrependReactor("update", "canaries", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("etcd is down")
	})

	changed, err := hasSpecChanged(logger, client, cd, targetSnapshot{hash: "v2:bbbb", fence: "uid-1/5"}, nil)
	require.NoError(t, err)
	assert.False(t, changed)
}

func TestHasSpecChanged_VolatileCounterSkipsFenceRefresh(t *testing.T) {
	cd, client, logger := specTestFixture(flaggerv1.CanaryStatus{
		Phase:               flaggerv1.CanaryPhaseSucceeded,
		LastAppliedSpec:     "v2:aaaa",
		LastPromotedSpec:    "v2:aaaa",
		LastTrackedRevision: "uid-1/500",
	})

	// a Service resourceVersion moves on writes unrelated to the spec;
	// chasing it would cost one status write per reconcile
	changed, err := hasSpecChanged(logger, client, cd,
		targetSnapshot{hash: "v2:aaaa", fence: "uid-1/501", volatileCounter: true}, nil)
	require.NoError(t, err)
	assert.False(t, changed)

	fresh := getSpecTestCanary(t, client)
	assert.Equal(t, "uid-1/500", fresh.Status.LastTrackedRevision)
}

func TestPersistSpecTracking_FreshnessRecheckedOnConflict(t *testing.T) {
	cd, client, _ := specTestFixture(flaggerv1.CanaryStatus{
		Phase:               flaggerv1.CanaryPhaseSucceeded,
		LastAppliedSpec:     "v2:aaaa",
		LastTrackedRevision: "uid-1/5",
	})

	// first status update hits a conflict; the retried GET observes a
	// concurrent writer's lastAppliedSpec — the retry must re-evaluate the
	// freshness check and abort instead of overwriting it
	conflicted := false
	injectConcurrentGet := false
	fakeClient := client.(*fakeFlagger.Clientset)
	fakeClient.PrependReactor("update", "canaries", func(k8stesting.Action) (bool, runtime.Object, error) {
		if !conflicted {
			conflicted = true
			injectConcurrentGet = true
			return true, nil, apierrors.NewConflict(
				schema.GroupResource{Group: "flagger.app", Resource: "canaries"}, "podinfo", errors.New("conflict"))
		}
		return false, nil, nil
	})
	fakeClient.PrependReactor("get", "canaries", func(k8stesting.Action) (bool, runtime.Object, error) {
		if injectConcurrentGet {
			concurrent := cd.DeepCopy()
			concurrent.Status.LastAppliedSpec = "v2:concurrent"
			concurrent.Status.LastTrackedRevision = "uid-1/9"
			return true, concurrent, nil
		}
		return false, nil, nil
	})

	require.NoError(t, persistSpecTracking(client, cd, targetSnapshot{hash: "v2:bbbb", fence: "uid-1/7"}))
	injectConcurrentGet = false

	// the stored canary was never overwritten by the aborted retry
	fresh := getSpecTestCanary(t, client)
	assert.Equal(t, "v2:aaaa", fresh.Status.LastAppliedSpec)
	assert.Equal(t, "uid-1/5", fresh.Status.LastTrackedRevision)
}

func TestRefreshTrackedRevision(t *testing.T) {
	cd, client, _ := specTestFixture(flaggerv1.CanaryStatus{
		Phase:               flaggerv1.CanaryPhaseSucceeded,
		LastAppliedSpec:     "v2:aaaa",
		LastTrackedRevision: "uid-1/5",
	})

	// hash matches the tracked spec: only the fence moves
	require.NoError(t, refreshTrackedRevision(client, cd, targetSnapshot{hash: "v2:aaaa", fence: "uid-1/7"}))
	fresh := getSpecTestCanary(t, client)
	assert.Equal(t, "uid-1/7", fresh.Status.LastTrackedRevision)

	// hash differs (concurrent template change captured by the scaling
	// write): the fence must stay put so the change is detected by hash
	require.NoError(t, refreshTrackedRevision(client, cd, targetSnapshot{hash: "v2:bbbb", fence: "uid-1/8"}))
	fresh = getSpecTestCanary(t, client)
	assert.Equal(t, "uid-1/7", fresh.Status.LastTrackedRevision)
	assert.Equal(t, "v2:aaaa", fresh.Status.LastAppliedSpec)
}

func TestDaemonSetSnapshot_ScaleToZeroSelectorIgnored(t *testing.T) {
	base := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "podinfo", Namespace: "default", UID: "uid-1", Generation: 3},
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "podinfo", Image: "img:1"}},
				},
			},
		},
	}

	scaledDown := base.DeepCopy()
	scaledDown.Spec.Template.Spec.NodeSelector = map[string]string{"flagger.app/scale-to-zero": "true"}

	baseSnap, err := daemonSetSnapshot(base)
	require.NoError(t, err)
	downSnap, err := daemonSetSnapshot(scaledDown)
	require.NoError(t, err)
	assert.Equal(t, baseSnap.hash, downSnap.hash)

	// user-provided node selectors are still part of the spec
	withSelector := base.DeepCopy()
	withSelector.Spec.Template.Spec.NodeSelector = map[string]string{"kubernetes.io/os": "linux"}
	selectorSnap, err := daemonSetSnapshot(withSelector)
	require.NoError(t, err)
	assert.NotEqual(t, baseSnap.hash, selectorSnap.hash)
}

func TestPodImagesDiffer(t *testing.T) {
	a := corev1.PodSpec{
		InitContainers: []corev1.Container{{Name: "setup", Image: "init:1"}},
		Containers:     []corev1.Container{{Name: "app", Image: "img:1"}},
	}

	b := *a.DeepCopy()
	assert.False(t, podImagesDiffer(a, b))

	b.Containers[0].Image = "img:2"
	assert.True(t, podImagesDiffer(a, b))

	c := *a.DeepCopy()
	c.InitContainers[0].Image = "init:2"
	assert.True(t, podImagesDiffer(a, c))

	d := *a.DeepCopy()
	d.Containers = append(d.Containers, corev1.Container{Name: "sidecar", Image: "sc:1"})
	assert.True(t, podImagesDiffer(a, d))
}

func TestFenceEncoding(t *testing.T) {
	assert.Equal(t, "uid-1/5", encodeFence("uid-1", "5", ""))
	assert.Equal(t, "uid-1/5/3", encodeFence("uid-1", "5", "3"))

	uid, counter, revision := parseFence("uid-1/5/3")
	assert.Equal(t, []string{"uid-1", "5", "3"}, []string{uid, counter, revision})

	uid, counter, revision = parseFence("uid-1/5")
	assert.Equal(t, []string{"uid-1", "5", ""}, []string{uid, counter, revision})

	uid, counter, revision = parseFence("")
	assert.Equal(t, []string{"", "", ""}, []string{uid, counter, revision})
}
