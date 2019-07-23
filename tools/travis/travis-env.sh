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
# Release Tag 
if [ "$TRAVIS_BRANCH" = "master" ]; then 
    RELEASE_TAG=latest 
else 
    RELEASE_TAG="${TRAVIS_BRANCH#release-}-latest" 
fi 
if [ "$TRAVIS_TAG" != "" ]; then 
    RELEASE_TAG="${TRAVIS_TAG#v}" 
fi 
export RELEASE_TAG="$RELEASE_TAG" 

# Release Tag 
echo TRAVIS_EVENT_TYPE=$TRAVIS_EVENT_TYPE 
echo TRAVIS_BRANCH=$TRAVIS_BRANCH 
echo TRAVIS_TAG=$TRAVIS_TAG 
echo RELEASE_TAG="$RELEASE_TAG" 