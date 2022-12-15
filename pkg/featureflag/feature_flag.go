/*
Copyright (c) Microsoft Corporation.
Licensed under the Apache 2.0 license.
*/
package featureflag

import (
	"context"

	"github.com/virtual-kubelet/virtual-kubelet/log"
)

const (
	InitContainerFeature = "init-container"
)

var enabledFeatures = []string{}

type FlagIdentifier struct {
	enabledFeatures []string
}

func InitFeatureFlag(ctx context.Context) *FlagIdentifier {
	log.G(ctx).Debug("loading enabled feature flags")

	var featureFlags FlagIdentifier
	featureFlags.enabledFeatures = enabledFeatures

	return &featureFlags
}

func (fi *FlagIdentifier) IsEnabled(ctx context.Context, feature string) bool {
	log.G(ctx).Debug("searching for %s in the enabled feature flags", feature)

	if fi.enabledFeatures == nil {
		log.G(ctx).Debug("no features is enabled")
		return false
	}
	for _, feat := range fi.enabledFeatures {
		if feat == feature {
			log.G(ctx).Infof("feature %s is enabled", feature)
			return true
		}
	}
	return false
}
