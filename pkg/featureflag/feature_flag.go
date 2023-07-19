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
	InitContainerFeature       = "init-container"
	ConfidentialComputeFeature = "confidential-compute"
	ManagedIdentityPullFeature = "managed-identity-image-pull"

	// Events : support ACI to K8s event translation and broadcasting
	Events = "events"
)

var enabledFeatures = []string{
	InitContainerFeature,
	ConfidentialComputeFeature,
	ManagedIdentityPullFeature,
	Events,
}

type FlagIdentifier struct {
	enabledFeatures []string
}

func InitFeatureFlag(ctx context.Context) *FlagIdentifier {
	log.G(ctx).Debug("loading enabled feature flags")

	var featureFlags FlagIdentifier
	featureFlags.enabledFeatures = enabledFeatures

	log.G(ctx).Infof("features %v enabled", enabledFeatures)

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
			log.G(ctx).Debugf("feature %s is enabled", feature)
			return true
		}
	}
	log.G(ctx).Debugf("feature %s is disabled", feature)
	return false
}
