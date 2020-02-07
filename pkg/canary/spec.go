package canary

import (
	"fmt"

	"github.com/mitchellh/hashstructure"
	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
)

func hasSpecChanged(cd *flaggerv1.Canary, spec interface{}) (bool, error) {
	if cd.Status.LastAppliedSpec == "" {
		return true, nil
	}

	newHash, err := hashstructure.Hash(spec, nil)
	if err != nil {
		return false, fmt.Errorf("hash error %v", err)
	}

	// do not trigger a canary deployment on manual rollback
	if cd.Status.LastPromotedSpec == fmt.Sprintf("%d", newHash) {
		return false, nil
	}

	if cd.Status.LastAppliedSpec != fmt.Sprintf("%d", newHash) {
		return true, nil
	}

	return false, nil
}
