package aci

import (
    "github.com/virtual-kubelet/virtual-kubelet/log"
    "context"
)

var minVersionSupport = map[string]string {
    "Priority": "2021-10-01",
}

// CurrentAssumption: version is decided based on values in containerGroup.Tags
// Set the minimum api version required based on the properties
// find the minimum version for each of the tags, based on known values
// find the max of the above min versions for each
func getAPIVersion(containerGroup ContainerGroup, ctx context.Context) string {
    finalApiVersion := apiVersion
    for key, val := range containerGroup.Tags {
        minVersion, ok := minVersionSupport[key]
        if len(val) == 0 || !ok {
            minVersion = apiVersion
        }
        if finalApiVersion < minVersion {
            finalApiVersion = minVersion
        }
    }
    log.G(ctx).Infof("setting api version to %s", finalApiVersion)
    return finalApiVersion
}


// TODO:
// pass podspec in create to allow for detailed versioning ??
// maintain minVersionsSupport in an external json ??
