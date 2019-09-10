/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.`
*/

package main

import (
	"flag"
	"os"
	"time"

	"github.com/ibm/composable/pkg/apis"
	"github.com/ibm/composable/pkg/controller"
	_ "k8s.io/client-go/plugin/pkg/client/auth" // Load all client auth plugins for GCP, Azure, Openstack, etc
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
)

func BuildConfig(kubeconfig *string) (*rest.Config, error) {
	var config *rest.Config
	var err error
	if *kubeconfig != "" { // off-cluster
		config, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
		if err != nil {
			return nil, err
		}
	} else { // in-cluster
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
	}
	return config, nil
}

func main() {

	klog.InitFlags(flag.CommandLine)
	kubeconfig := flag.String("kubeconfig", "", "Path to a kube config. Only required if out-of-cluster.")
	if flag.Lookup("kubeconfig") == nil {
		flag.String("kubeconfig", os.Getenv("KUBECONFIG"), "Path to a kube config. Only required if out-of-cluster.")
	}
	syncPeriod := flag.Duration("syncPeriod", 30 * time.Second, "Defines the minimum frequency at which watched Compsable resources are reconciled." )
	flag.Parse()
	// build config for the  cluster
	cfg, err := BuildConfig(kubeconfig)
	if err != nil {
		klog.Fatalf("BuildConfig returned error: %q", err.Error())
	}

	// Get a config to talk to the apiserver
	//cfg, err := config.GetConfig()
	//if err != nil {
	//	log.Fatal(err)
	//}

	// Create a new Cmd to provide shared dependencies and start components
	mgr, err := manager.New(cfg, manager.Options{SyncPeriod: syncPeriod})
	if err != nil {
		klog.Fatalf("manager.New returned error: %q", err.Error())
	}

	klog.V(3).Infoln("Registering Components.")

	// Setup Scheme for all resources
	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		klog.Fatalf("apis.AddToScheme returned error: %q", err.Error())
	}

	// Setup all Controllers
	if err := controller.AddToManager(mgr); err != nil {
		klog.Fatalf("controller.AddToManager returned error: %q", err.Error())
	}

	klog.V(3).Infoln("Starting the Cmd.")

	// Start the Cmd
	klog.Error(mgr.Start(signals.SetupSignalHandler()))
}
