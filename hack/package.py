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

import argparse
import os
import sys
import re
import shutil
import yaml
import datetime
import json
from collections import OrderedDict


parser = argparse.ArgumentParser(description='Package bundle for OperatorHub')
parser.add_argument('version', type=str, nargs='?', metavar='version', help='version to package')
parser.add_argument('--is_update', action='store_true', default=False, dest='is_update', help='version update')
args = parser.parse_args()

if args.version == None:
    print("Usage: python"+os.path.basename(__file__)+" <version> <--is_update>")
    sys.exit()

################################
## Utilities
################################

def rename_crd(crdname):
    tk = re.split('[_.]', crdname)
    return tk[1]+"_operator_"+tk[2]+"_"+tk[3]+".crd.yaml"

def find_deployment(source):
    for filename in os.listdir(source):
        if (filename.find("deployment")>=0):
            return filename
    print("Could not find deployment file in %s",source)     
    sys.exit()  

def find_role(source):
    for filename in os.listdir(source):
        if (filename.find("role.yaml")>=0):
            return filename
    print("Could not find role file in %s",source)     
    sys.exit()  

# returns >0 if semantic version v1 > v2, = 0 if v1 == v2, < 0 is v1 < v2
def version_is_greater(v1, v2):
    v1s = v1[1:].split('.')
    v2s = v2[1:].split('.')    
    v1v = int(v1s[0])*8 + int(v1s[1])*4 + int(v1s[2])*2
    v2v = int(v2s[0])*8 + int(v2s[1])*4 + int(v2s[2])*2
    return v1v - v2v

# allow to insert literals (|-) in yaml
class literal(str):
    pass

def literal_presenter(dumper, data):
    return dumper.represent_scalar('tag:yaml.org,2002:str', data, style='|')
yaml.add_representer(literal, literal_presenter)

def ordered_dict_presenter(dumper, data):
    return dumper.represent_dict(data.items())


script_home=os.path.dirname(os.path.realpath(__file__))
os.chdir(script_home)
config = os.path.join(script_home,"..","config")
releases=os.path.join(script_home,"..","releases",args.version)
olm=os.path.join(script_home,"..","olm",args.version[1:])
latest=os.path.join(script_home,"..","releases","latest")

# load defaults 
with open(os.path.join(config,"templates","defaults.yaml"), 'r') as stream:
    defs=yaml.safe_load(stream)

# generate the directories for version if not already existing & create symlink for latest
if not os.path.exists(releases):
    os.makedirs(releases)    
if not os.path.exists(olm):
    os.makedirs(olm)        
if os.path.exists(latest):
    os.unlink(latest)
os.symlink(os.path.join("..","releases",args.version), os.path.join("..","releases","latest"))      

################################
## Generate Release Yaml
################################

# copy namespace
ix = 0
ns = "namespace.yaml"
new_name = "%03d_%s" % (ix,ns)
shutil.copyfile(os.path.join(config,"templates",ns),os.path.join(releases,new_name))
ix += 1

# copy crds
crds = os.path.join(config,"crd", "bases")
for filename in os.listdir(crds):
    new_name = "%03d_%s" % (ix,filename)
    shutil.copyfile(os.path.join(crds,filename),os.path.join(releases,new_name))
    ix += 1

# copy service account
sa = "serviceaccount.yaml"
new_name = "%03d_%s" % (ix,sa)
shutil.copyfile(os.path.join(config,"templates",sa),os.path.join(releases,new_name))
ix += 1

# copy rbac_role
rbac_role_file = "manager_role.yaml"
new_name = "%03d_%s" % (ix,rbac_role_file)
# load rbac_role from kubebuilder
with open(os.path.join(config,"rbac",rbac_role_file), 'r') as rbacstream:
    rbac_role = yaml.safe_load(rbacstream)
# rename
rbac_role['metadata']['name'] =  "composable-operator-manager-role"  
# write it back with new name
with open(os.path.join(releases,new_name), "w") as outfile:
        yaml.dump(rbac_role, outfile, default_flow_style=False)
ix += 1

# copy role binding
rb = "rbac_role_binding.yaml"
new_name = "%03d_%s" % (ix,rb)
shutil.copyfile(os.path.join(config,"templates",rb),os.path.join(releases,new_name))
ix += 1

# copy deployment file
deploy_file = "deployment.yaml"
new_name = "%03d_%s" % (ix,deploy_file)
# load rbac_role from kubebuilder
with open(os.path.join(config,"templates",deploy_file), 'r') as deploystream:
    deploy = yaml.safe_load(deploystream)
# update image
deploy['spec']['template']['spec']['containers'][0]['image'] = defs['image']+":"+args.version[1:]
# write it back with new name
with open(os.path.join(releases,new_name), "w") as outfile:
        yaml.dump(deploy, outfile, default_flow_style=False)
ix += 1


################################
## Generate OperatorHub metadata
################################

# if it is an update, check if previous releases exist & get replaced version
if args.is_update:
    previous_version_exists = False
    min = sys.maxsize
    hashv = {}
    for filename in os.listdir(os.path.join(script_home,"..","releases")):
        if(filename == "latest"):
            continue
        vx = version_is_greater(args.version,filename)
        if ( vx <0 ):
            print("Must provide semantic version >= to existing versions")
            sys.exit()   

        if (vx >0 ):
            previous_version_exists = True
            if (vx < min):
                min = vx
                hashv[min] = filename

    if (not previous_version_exists):
        print("No valid previous version exists")
        sys.exit()
    replaced_version = hashv[min]  

# iterate sources
for filename in os.listdir(releases):
    # we want only crds
    if (filename.find("ibmcloud.ibm.com")<0):
        continue
    shutil.copyfile(os.path.join(releases,filename),os.path.join(olm,rename_crd(filename)))

# write/update package file
with open(os.path.join(config,"templates","template.package.yaml"), 'r') as stream:
    pkg=yaml.safe_load(stream)
    pkg['channels'][0]['currentCSV'] = defs['operator_name']+"."+args.version
    pkg['channels'][0]['name'] = defs['channel_name']
    pkg['packageName'] = defs['operator_name']

    with open(os.path.join(olm,"..","composable_operator.package.yaml"), "w") as outfile:
        yaml.dump(pkg, outfile, default_flow_style=False)
 
# fill in cluster service version from template, deployment and roles
with open(os.path.join(config,"templates","template.clusterserviceversion.yaml"), 'r') as stream:
    csv=yaml.safe_load(stream)

    # get deployment
    deploy_file = find_deployment(releases)
    with open(os.path.join(releases,deploy_file), 'r') as depstream:
        deploy=yaml.safe_load(depstream)

    # get roles
    role_file = find_role(releases)
    with open(os.path.join(releases,role_file), 'r') as rolestream:
        role=yaml.safe_load(rolestream)

    # set csv fields 
    containerImage = deploy['spec']['template']['spec']['containers'][0]['image']
    csv['metadata']['annotations']['containerImage'] = containerImage

    now = datetime.datetime.now()                              
    csv['metadata']['annotations']['createdAt'] = now.strftime('%Y-%m-%dT%H:%M:%SZ')

    csv['metadata']['name'] = defs['operator_name']+"."+args.version

    csv['spec']['install']['spec']['clusterPermissions'][0]['rules'] = role['rules']

    sa = deploy['spec']['template']['spec']['serviceAccountName']
    csv['spec']['install']['spec']['clusterPermissions'][0]['serviceAccountName'] = sa

    csv['spec']['install']['spec']['deployments'][0]['spec'] = deploy['spec']
    csv['spec']['install']['spec']['deployments'][0]['name'] = defs['operator_name']

    csv['spec']['maturity'] = defs['maturity']
    csv['spec']['labels']['name'] = defs['operator_name']
    csv['spec']['selector']['matchLabels']['name'] = defs['operator_name']

    csv['spec']['version'] = args.version[1:]
    
    # iterate crds to fill in crd fields
    # first load yaml in hashmap:
    crdmap = {}
    for filename in os.listdir(releases):
        # we want only crds
        if (filename.find("composables")<0):
            continue
        # load yaml
        with open(os.path.join(releases,filename), 'r') as crdstream:
            crd=yaml.safe_load(crdstream)
            kind = crd['spec']['names']['kind']
            crdmap[kind] = crd


    # we want only to package the crds defined in the defaults
    # so we iterate on those
    csv['spec']['customresourcedefinitions']['owned'] = []
    alm_examples = []
    for i in range(len(defs['crd'])):
        # get crd from hashmap
        kind = defs['crd'][i]['kind']
        crd = crdmap.get(kind)
        if crd != None:
            description = defs['crd'][i]['description']
            crd_name = crd['metadata']['name']
            crd_version = crd['spec']['version']
            # fill in csv
            owned = {}
            owned['description'] = description
            owned['displayName'] = kind
            owned['kind'] = kind
            owned['name'] = crd_name
            owned['version'] = crd_version
            owned['resources'] =  defs['crd'][i]['resources']
            owned['specDescriptors'] =  defs['crd'][i]['specDescriptors']
            owned['statusDescriptors'] =  defs['crd'][i]['statusDescriptors']
            csv['spec']['customresourcedefinitions']['owned'].append(owned)
            # add examples
            ex = json.loads(defs['crd'][i]['example'].decode('utf-8'), object_pairs_hook=OrderedDict)
            alm_examples.append(ex)
        else:
            print("WARNING: kind %s not found!" % kind)    
                   
    csv['metadata']['annotations']['alm-examples'] = literal(json.dumps(alm_examples))

    if args.is_update and replaced_version:
        csv['spec']['replaces'] = defs['operator_name']+"."+replaced_version

    with open(os.path.join(olm,"composable_operator."+args.version+".clusterserviceversion.yaml"), "w") as outfile:
        yaml.dump(csv, outfile, default_flow_style=False)

with open(os.path.join(script_home,"latest_tag"), "w") as f:
    f.write("export TAG=%s" % (args.version[1:])) 
    f.close()