package aci

import (
    "github.com/virtual-kubelet/virtual-kubelet/log"
    "context"
)

//<summary>
//	map of minimum api version required for different properties
//	TODO: read from a separate json file if it gets too large
//</summary>
var minVersionSupport = map[string]string {
    "ConfidentialCompute": "2022-04-01-preview",
}

//<summary>
//	VersionProvider struct for selecting api version based on various properties
//	Maintains the api version selected after checking certain property
//<summary>
type VersionProvider struct {
    finalVersion string
}

//<summary>
//	creates a new instance of VErsionProvider, and sets default version
//	assumes api version to be of the format YYYY-mm-dd[-suffix]
//	assumes that the api version format will not be violated
//</summary>
//<param name="defaultVersion"> The default api version </param>
//<returns>
//	reference to  an instance of the verison provider object
//</returns>
func newVersionProvider(defaultVersion string) (*VersionProvider) {
    return &VersionProvider{defaultVersion}
}

//<summary>
//	get the api version for the specific ContainerGroup instance based on various properties
//</summary>
//<param name="containerGroup"> the ContainerGroup instance for which version is to be selected </param>
//<param name="ctx"> the Context to be used for logging </param>
//<returns>
//	reference to an instance of VersionProvider with the finalVersion field updated
//</returns>
func (versionProvider *VersionProvider) getVersion(containerGroup ContainerGroup, ctx context.Context) (* VersionProvider) {
	if containerGroup.ContainerGroupProperties.ConfidentialComputeProperties != nil {
		versionProvider.setVersionFromProperty(string(containerGroup.ContainerGroupProperties.ConfidentialComputeProperties.CCEPolicy), "ConfidentialCompute", ctx)
	}
    log.G(ctx).Infof("API Version set to %s \n", versionProvider.finalVersion)
    return versionProvider
}

//<summary>
//	find the min api version for a string property based on the value in minVersionSupport map
//	assumes that the api version always uses the correct format YYYY-mm-dd[-suffix]
//</summary>
//<param name="property">the string value of some property field that should be checked<param>
//<param name="keyRef">the key for the property in the minVersionSupport map</param>
//<param name="ctx">the context to be used for logging</param>
//<returns>
//	reference to an instance of VersionProvider with the finalVersion field updated
//</returns>
func (versionProvider *VersionProvider) setVersionFromProperty(property string, keyRef string, ctx context.Context) (*VersionProvider) {
    minVersion, minVersionExists := minVersionSupport[keyRef]
    if len(property) > 0 && minVersionExists && versionProvider.finalVersion < minVersion {
        versionProvider.finalVersion = minVersion
    }
    log.G(ctx).Infof("Selected API Version %s for property %s with value %s \n", versionProvider.finalVersion, keyRef, property)
    return versionProvider
}
