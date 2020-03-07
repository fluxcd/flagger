package canary

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeploymentController_IsReady(t *testing.T) {
	mocks := newDeploymentFixture()
	err := mocks.controller.Initialize(mocks.canary, true)
	require.NoError(t, err, "Expected primary readiness check to fail")

	err = mocks.controller.IsPrimaryReady(mocks.canary)
	require.Error(t, err)

	_, err = mocks.controller.IsCanaryReady(mocks.canary)
	require.NoError(t, err)
}
