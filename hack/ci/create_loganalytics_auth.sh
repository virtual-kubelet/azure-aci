#!/bin/bash

set -e -u

mkdir -p $(dirname "${TEST_LOGANALYTICS_JSON}")

# This will build the log analytics credentials during CI
cat <<EOF > "${TEST_LOGANALYTICS_JSON}"
{
  "workspaceId": "######-###-####-####-######",
  "workspaceKey": "######-###-####-####-######"
}
EOF
