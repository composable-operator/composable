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




set -e

#
# This will clone the composable-operator/composable git repo to the director
# where this script is located, and install the kustomize resources in config/default
#

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"

gitdir="${SCRIPT_DIR}/composable"

if [[ ! -d "${gitdir}" ]]; then
  git -C "${SCRIPT_DIR}" clone git@github.com:composable-operator/composable.git
else
  git -C "${gitdir}" pull
fi

echo "Installing composable-operator"

kubectl apply -k "${gitdir}/config/default/"
