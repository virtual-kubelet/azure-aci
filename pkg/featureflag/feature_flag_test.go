package featureflag

import (
	"context"
	"fmt"
	"testing"

	"gotest.tools/assert"
)

func TestIsEnabled(t *testing.T) {

	cases := []struct {
		description   string
		feature       string
		shouldEnabled bool
	}{
		{
			description:   fmt.Sprintf(" %s feature should be enabled", InitContainerFeature),
			feature:       InitContainerFeature,
			shouldEnabled: true,
		},
		{
			description:   fmt.Sprintf(" %s feature should be enabled", ConfidentialComputeFeature),
			feature:       ConfidentialComputeFeature,
			shouldEnabled: true,
		},
	}
	for i, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
			ctx := context.TODO()

			featId := InitFeatureFlag(ctx)
			result := featId.IsEnabled(ctx, tc.feature)
			assert.Equal(t, result, tc.shouldEnabled, "test[%d]", i)
		})
	}
}
