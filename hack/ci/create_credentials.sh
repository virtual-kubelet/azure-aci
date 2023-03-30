#!/bin/bash

set -e -u

mkdir -p $(dirname "${TEST_CREDENTIALS_JSON}")

# This will build the credentials during the CI
cat <<EOF > "${TEST_CREDENTIALS_JSON}"
{
  "clientId": "######-###-####-####-######",
  "clientSecret": "######-###-####-####-######",
  "subscriptionId": "######-###-####-####-######",
  "tenantId": "######-###-####-####-######",
  "activeDirectoryEndpointUrl": "######-###-####-####-######",
  "resourceManagerEndpointUrl": "######-###-####-####-######",
  "activeDirectoryGraphResourceId": "######-###-####-####-######",
  "sqlManagementEndpointUrl": "######-###-####-####-######",
  "galleryEndpointUrl": "######-###-####-####-######",
  "managementEndpointUrl": "######-###-####-####-######"
}
EOF
