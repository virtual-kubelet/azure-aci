#!/bin/bash

set -e -u

mkdir -p $(dirname "${TEST_AKS_CREDENTIALS_JSON}")

# This will build the AKS credentials during the CI
cat <<EOF > "${TEST_AKS_CREDENTIALS_JSON}"
{
   "cloud": "AzurePublicCloud",
   "tenantId": "######-###-####-####-######",
   "subscriptionId": "######-###-####-####-######",
   "aadClientId": "msi",
   "aadClientSecret": "msi",
   "resourceGroup": "######-###-####-####-######",
   "location": "centralus",
   "vnetName": "#####",
   "vnetResourceGroup": "######-###-####-####-######",
   "userAssignedIdentityID": "######-###-####-####-######"
}
EOF
