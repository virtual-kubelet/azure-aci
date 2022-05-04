package aci

import (
    "github.com/virtual-kubelet/virtual-kubelet/log"
    "context"
    "regexp"
    "fmt"
)

// map of minimum api versions required for different properties
// TODO: read from a separate json file if it gets too large
var minVersionSupport = map[string]string {
    "Priority": "2021-10-01",
}

// Basic object for selecting api version based on various properties
// Maintains the api version selected after checking certain properties
type VersionProvider struct {
    finalVersion string
}

// creates a new instance of VersionProvider, and sets the default version
// assumes api version to be of the form YYYY-mm-dd
// doesn't create an object with invalid version
func newVersionProvider(defaultVersion string) (*VersionProvider, error) {
    if res, _ := isValidVersion(defaultVersion); res {
        return &VersionProvider{defaultVersion}, nil
    }
    return nil, fmt.Errorf("Version %s doesn't follow YYYY-mm-dd format", defaultVersion)
}

// returns the api version for the specific ContainerGroup instance based on verious properties
func (versionProvider *VersionProvider) getVersion(containerGroup ContainerGroup, ctx context.Context) (* VersionProvider) {

    versionProvider.setVersionFromProperty(string(containerGroup.ContainerGroupProperties.Priority), "Priority")

    log.G(ctx).Infof("API Version set to %s \n", versionProvider.finalVersion)
    return versionProvider
}

// find the min api version for a string property based on the value in minVersionSupport map
// don't update the api version unless the api version in map is valid
func (versionProvider *VersionProvider) setVersionFromProperty(property string, keyRef string) (*VersionProvider) {
    minVersion, ok := minVersionSupport[keyRef]
    if len(property) > 0 && ok && versionProvider.finalVersion < minVersion {
        res, err := isValidVersion(minVersion)
        if err == nil && res {
            versionProvider.finalVersion = minVersion
        }
    }
    return versionProvider
}

// validate the format of version
// assumes version is in YYYY-mm-dd format and hence sortable
func isValidVersion(version string) (bool, error) {
    exp := "[0-9]{4}-[0-9]{2}-[0-9]{2}"
    res, err := regexp.MatchString(exp, version)
    if err != nil {
        return false, fmt.Errorf("Error validating version %s", version)
    }
    return res, nil
}

// TODO: PR
// 1. add unit tests for getVersion and setVersionFromProperty
