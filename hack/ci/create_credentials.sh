#!/bin/bash

set -e -u

mkdir -p $(dirname "${TEST_CREDENTIALS_JSON}")

echo $clientId

# This will build the credentials during the CI
cat <<EOF > "${TEST_CREDENTIALS_JSON}"
{
  "clientId": "$clientId",
  "clientSecret": "$clientSecret",
  "subscriptionId": "$subscriptionId",
  "tenantId": "$tenantId",
  "activeDirectoryEndpointUrl": "$activeDirectoryEndpointUrl",
  "resourceManagerEndpointUrl": "$resourceManagerEndpointUrl",
  "activeDirectoryGraphResourceId": "$activeDirectoryGraphResourceId",
  "sqlManagementEndpointUrl": "$sqlManagementEndpointUrl",
  "galleryEndpointUrl": "$galleryEndpointUrl",
  "managementEndpointUrl": "$managementEndpointUrl"
}
EOF
