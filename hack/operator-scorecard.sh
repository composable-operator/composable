#!/bin/bash
#
# Copyright 2019 IBM Corp. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

OPERATOR_SDK_CMD=$(command -v operator-sdk)
TREE_CMD=$(command -v tree)
SCRIPTS_HOME=$(dirname ${BASH_SOURCE})

if [ -z ${OPERATOR_SDK_CMD} ]; then
    echo "'operator-sdk' command not found, exiting."
    exit 1
fi    

source ${SCRIPTS_HOME}/latest_tag

# create a test deployment dir in the format expected by operator-sdk scorecard and copy artifacts there
TEST_DEPLOY_DIR=`mktemp -d 2>/dev/null || mktemp -d -t 'mytmpdir'`

echo "Copying artifacts in $TEST_DEPLOY_DIR ..."
mkdir -p ${TEST_DEPLOY_DIR}/deploy/crds
cp ${SCRIPTS_HOME}/../config/crd/bases/ibmcloud.ibm.com_composables.yaml ${TEST_DEPLOY_DIR}/deploy/crds/ibmcloud_v1alpha1_composable_crd.yaml
cp ${SCRIPTS_HOME}/../config/samples/comp3.yaml ${TEST_DEPLOY_DIR}/deploy/crds/comp_cr.yaml

# now copy all other artifacts
cp ${SCRIPTS_HOME}/../releases/latest/*_serviceaccount.yaml ${TEST_DEPLOY_DIR}/deploy/service_account.yaml
cp ${SCRIPTS_HOME}/../releases/latest/*_role_binding.yaml ${TEST_DEPLOY_DIR}/deploy/role_binding.yaml
cp ${SCRIPTS_HOME}/../releases/latest/*_role.yaml ${TEST_DEPLOY_DIR}/deploy/role.yaml
cp ${SCRIPTS_HOME}/../releases/latest/*_deployment.yaml ${TEST_DEPLOY_DIR}/deploy/operator.yaml
cp ${SCRIPTS_HOME}/../olm/v${TAG}/composable_operator.v${TAG}.clusterserviceversion.yaml ${TEST_DEPLOY_DIR}/deploy/composable_operator.v${TAG}.clusterserviceversion.yaml

if [ -z ${TREE_CMD} ]; then
    echo "'tree' command not found, consider installing it."
else
    tree $TEST_DEPLOY_DIR
fi    

# launch the scorecard
NS=$(cat ${SCRIPTS_HOME}/../releases/latest/*deployment.yaml | grep namespace | awk '{print $2}')

# create install ns if needed
kubectl get ns ${NS} >/dev/null 2>&1
if [ $? -eq 0 ]
then
    echo "Namespace ${NS} already exists"
else
    echo "Creating namespace ${NS}"
    kubectl create ns ${NS} 
fi

cd $TEST_DEPLOY_DIR
operator-sdk scorecard \
#--cr-manifest deploy/crds/service_cr.yaml \
#--cr-manifest deploy/crds/binding_cr.yaml \
#--csv-path deploy/ibmcloud_operator.v${TAG}.clusterserviceversion.yaml \
--namespace ${NS}

# clean up
rm -r $TEST_DEPLOY_DIR
