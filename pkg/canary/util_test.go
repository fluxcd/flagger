package canary

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIncludeLabelsByPrefix(t *testing.T) {
	labels := map[string]string{
		"foo":   "foo-value",
		"bar":   "bar-value",
		"lorem": "ipsum",
	}
	includeLabelPrefix := []string{"foo", "lor"}

	filteredLabels := includeLabelsByPrefix(labels, includeLabelPrefix)

	assert.Equal(t, filteredLabels, map[string]string{
		"foo":   "foo-value",
		"lorem": "ipsum",
		// bar excluded
	})
}

func TestIncludeLabelsByPrefixWithWildcard(t *testing.T) {
	labels := map[string]string{
		"foo":   "foo-value",
		"bar":   "bar-value",
		"lorem": "ipsum",
	}
	includeLabelPrefix := []string{"*"}

	filteredLabels := includeLabelsByPrefix(labels, includeLabelPrefix)

	assert.Equal(t, filteredLabels, map[string]string{
		"foo":   "foo-value",
		"bar":   "bar-value",
		"lorem": "ipsum",
	})
}

func TestMakePrimaryLabels(t *testing.T) {
	labels := map[string]string{
		"lorem": "ipsum",
		"foo":   "old-bar",
	}

	primaryLabels := makePrimaryLabels(labels, "new-bar", "foo")

	assert.Equal(t, primaryLabels, map[string]string{
		"lorem": "ipsum",   // values from old map
		"foo":   "new-bar", // overriden value for a specific label
	})
}
