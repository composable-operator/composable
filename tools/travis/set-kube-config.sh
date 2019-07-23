#!/usr/bin/env bash
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


if [ -z "${KUBECONFIG_B64}" ] || [ -z "${KUBECONFIG_CA_CERT_B64}" ] ; then
    echo "KUBECONFIG_B64 or KUBECONFIG_CA_CERT_B64" are not set, skipping kube configuration
    exit 0
fi

mkdir -p ~/.kube
echo $KUBECONFIG_B64 | base64 --decode > ${HOME}/.kube/config
echo $KUBECONFIG_CA_CERT_B64 | base64 --decode > ${HOME}/.kube/ca.pem
