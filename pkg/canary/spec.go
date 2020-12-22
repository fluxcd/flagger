package canary

import (
	"fmt"
	"hash/fnv"

	"github.com/davecgh/go-spew/spew"
	"k8s.io/apimachinery/pkg/util/rand"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
)

// hasSpecChanged computes the hash of the spec and compares it with the
// last applied spec, if the last applied hash is different but not equal
// to last promoted one the it returns true
func hasSpecChanged(cd *flaggerv1.Canary, spec interface{}) (bool, error) {
	if cd.Status.LastAppliedSpec == "" {
		return true, nil
	}

	newHash := computeHash(spec)

	// do not trigger a canary deployment on manual rollback
	if cd.Status.LastPromotedSpec == newHash {
		return false, nil
	}

	if cd.Status.LastAppliedSpec != newHash {
		return true, nil
	}

	return false, nil
}

// computeHash returns a hash value calculated from a spec using the spew library
// which follows pointers and prints actual values of the nested objects
// ensuring the hash does not change when a pointer changes.
func computeHash(spec interface{}) string {
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
