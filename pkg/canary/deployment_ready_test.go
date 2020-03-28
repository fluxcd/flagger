package canary

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeploymentController_IsReady(t *testing.T) {
	mocks := newDeploymentFixture()
	mocks.controller.Initialize(mocks.canary)

	err := mocks.controller.IsPrimaryReady(mocks.canary)
	require.Error(t, err)

	_, err = mocks.controller.IsCanaryReady(mocks.canary)
	require.NoError(t, err)
}

// TODO: more detailed tests as daemonset
