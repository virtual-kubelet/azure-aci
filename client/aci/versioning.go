package aci

import (
    "github.com/virtual-kubelet/virtual-kubelet/log"
    "context"
)

// map of minimum api version required for different properties
// TODO: read from a separate json file if it gets too large
var minVersionSupport = map[string]string {
    "Priority": "2021-10-01",
}

// Basic object for selecting api version based on various properties
// Maintains the api version selected after checking certain properties
type VersionProvider struct {
    finalVersion string
}

// creates a new instance of VErsionProvider, and sets default version
// assumes api version to be of the format YYYY-mm-dd[-suffix]
// assumes that the api version format will not be violated
func newVersionProvider(defaultVersion string) (*VersionProvider) {
    return &VersionProvider{defaultVersion}
}

// returns the api version for the specific ContainerGroup instance based on various properties
func (versionProvider *VersionProvider) getVersion(containerGroup ContainerGroup, ctx context.Context) (* VersionProvider) {

    versionProvider.setVersionFromProperty(string(containerGroup.ContainerGroupProperties.Priority), "Priority")

    log.G(ctx).Infof("API Version set to %s \n", versionProvider.finalVersion)
    return  versionProvider
}

// find the min api version for a string property based on the value in minVersionSupport map
// assumes that the api version always uses the correct format YYYY-mm-dd[-suffix]
func (versionProvider *VersionProvider) setVersionFromProperty(property string, keyRef string) (*VersionProvider) {
    minVersion, ok := minVersionSupport[keyRef]
    if len(property) > 0 && ok && versionProvider.finalVersion < minVersion {
        versionProvider.finalVersion = minVersion
    }
    return versionProvider
}
