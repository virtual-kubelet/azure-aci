package aci

import (
    "github.com/virtual-kubelet/virtual-kubelet/log"
    "context"
)

var minVersionSupport = map[string]string {
    "Priority": "2021-10-01",
}

type VersionProvider struct {
    finalVersion string
    ctx context.Context
}

func newVersionProvider(defaultVersion string, ctx context.Context) (*VersionProvider) {
    return &VersionProvider{defaultVersion, ctx}
}

// call check version based on some property
// return the version Provider with finalVersion updated 
// keep adding more checks under this in future
func (versionProvider *VersionProvider) getVersion(containerGroup ContainerGroup) (* VersionProvider) {

    versionProvider.setVersionFromProperty(string(containerGroup.ContainerGroupProperties.Priority), "Priority")

    log.G(versionProvider.ctx).Infof("API Version set to %s \n", versionProvider.finalVersion)
    return  versionProvider
}

// find the minimum version for a property from the map
func (versionProvider *VersionProvider) setVersionFromProperty(property string, keyRef string) (*VersionProvider) {
    minVersion, ok := minVersionSupport[keyRef]
    if len(property) > 0 && ok && versionProvider.finalVersion < minVersion {
        versionProvider.finalVersion = minVersion
    }
    return versionProvider
}

// TODO:
// maintain minVersionsSupport in an external json ??
