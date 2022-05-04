package aci

import (
	"testing"
	"context"
	"gotest.tools/assert"
	is "gotest.tools/assert/cmp"
)

// tests setlecting version when property is present in map
func TestSetVersionFromPropertyInMap(t *testing.T) {
	key := "Priority"
	defaultVersion := apiVersion
	versionProvider:= newVersionProvider(defaultVersion)

	// should use defualt version when property is not populated
	versionProvider.setVersionFromProperty("", key)
	assert.Check(t, is.Equal(versionProvider.finalVersion, defaultVersion), "Default version should be used when a property value is empty")

	// should use api version present in map if priority is Spot
	versionProvider.setVersionFromProperty("Spot", key)
	assert.Check(t, is.Equal(versionProvider.finalVersion, minVersionSupport[key]), "When a version is available for the property in map, the final version should be >= min api version for the property")

	// should use api version present in map if priority is Regular
	versionProvider.setVersionFromProperty("Regular", key)
	assert.Check(t, is.Equal(versionProvider.finalVersion, minVersionSupport[key]), "When a version is available for the property in map, the final version should be >= min api version for the property")

}

// tests selecting version when a property is present in map with version < defualtVersion
func TestSetLowerVersionFromPropertyInMap (t *testing.T) {
	key := "Priority"
	largeDefaultVersion := "9999-99-99"
	versionProvider := newVersionProvider(largeDefaultVersion)

	// should use largeDefaultVersion as it is > the min api version for thiis key
	versionProvider.setVersionFromProperty("Regular", key)
	assert.Check(t, versionProvider.finalVersion >= minVersionSupport[key], "Use larger version among default and min api versions for various properties")
}

// test selecting version when property is not present in map
func TestSetVersionFromPropertyNotInMap(t *testing.T) {
	key := "someUnknownKey"
	defaultVersion := apiVersion
	versionProvider:= newVersionProvider(defaultVersion)

	// should use default version when property is not present in map
	versionProvider.setVersionFromProperty("propertyValue", key)
	assert.Check(t, versionProvider.finalVersion == defaultVersion, "Default version should be used when no version for a property is available in map")
}

// test getVersion for ContainerGroup with Priority
func TestGetVersionForContainerGroupWithPriority(t *testing.T) {
	key := "Priority"
	versionProvider := newVersionProvider(apiVersion)
	containerGroup := ContainerGroup{
		Location: "eastus",
		ContainerGroupProperties: ContainerGroupProperties{
			Priority: Spot,
			OsType: Linux,
		},
	}

	// should use api version in map when priority is set for a containerGroup
	versionProvider.getVersion(containerGroup, context.Background())
	assert.Check(t, is.Equal(versionProvider.finalVersion, minVersionSupport[key]), "Use api version in map when priority is set for a containerGroup")

}

// test getVersion for containerGroup without Priority
func TestGetVersionForContainerGroupWithoutPriority(t *testing.T) {
	defaultVersion := apiVersion
	versionProvider := newVersionProvider(defaultVersion)
	containerGroup := ContainerGroup{
		Location: "eastus",
		ContainerGroupProperties: ContainerGroupProperties{
			OsType: Linux,
		},
	}

	// should use default api version when priority is not set for a containerGroup
	versionProvider.getVersion(containerGroup, context.Background())
	assert.Check(t, is.Equal(versionProvider.finalVersion, defaultVersion), "Use default api version when priority is not set for a containerGroup")

}
