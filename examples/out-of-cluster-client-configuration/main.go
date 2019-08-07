/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Note: the example only works with the code within the same release/branch.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/clientcmd"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"

	//
	// Uncomment to load all auth plugins
	// _ "k8s.io/client-go/plugin/pkg/client/auth"
	//
	// Or uncomment to load specific auth plugins
	// _ "k8s.io/client-go/plugin/pkg/client/auth/azure"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	// _ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	// _ "k8s.io/client-go/plugin/pkg/client/auth/openstack"
)

func main() {
	var kubeconfig *string
	if home := homeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	// create the clientset

	dynamicClient, _ := dynamic.NewForConfig(config)

	if err != nil {
		panic(err.Error())
	}

	destrinationGVR := schema.GroupVersionResource{Group: "networking.istio.io", Version: "v1alpha3", Resource: "destinationrules"}
	drDynamicInformer := dynamicinformer.NewFilteredDynamicInformer(dynamicClient, destrinationGVR, "", 0,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
		nil)

	stopCh := make(chan struct{})

	go drDynamicInformer.Informer().Run(stopCh)

	drDynamicInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			fmt.Println("CRD DestinationRule ADD")
			drus := obj.(*unstructured.Unstructured)
			fmt.Printf("\t Name: %s\n", drus.GetName())
			fmt.Printf("\t Host: %s\n", drus.Object["spec"])

		},
		UpdateFunc: func(old, cur interface{}) {
			fmt.Println("CRD update.")
			drus := cur.(*unstructured.Unstructured)
			annotations := drus.GetAnnotations()
			if _, ok := annotations["neg-status"]; !ok {
				newDSus := drus.DeepCopy()
				annotations["neg-status"] = "Hiiiiiiii"
				newDSus.SetAnnotations(annotations)

				patchBytes, _ := StrategicMergePatchBytes(drus, newDSus, unstructured.Unstructured{})
				fmt.Printf("CRD NEG Annotation updated, diff: %s\n", string(patchBytes))
				//dynamicClient.Resource(destrinationGVR).Patch(newDSus.GetName(), types.StrategicMergePatchType, patchBytes, metav1.PatchOptions{})
				_, err := dynamicClient.Resource(destrinationGVR).Namespace(newDSus.GetNamespace()).Update(newDSus, metav1.UpdateOptions{})
				if err != nil {
					fmt.Printf("Update error: %s\n", err)
				}
			}
		},
	})

	<-stopCh
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}

func StrategicMergePatchBytes(old, cur, refStruct interface{}) ([]byte, error) {
	oldBytes, err := json.Marshal(old)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal old object: %v", err)
	}

	newBytes, err := json.Marshal(cur)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal new object: %v", err)
	}

	patchBytes, err := strategicpatch.CreateTwoWayMergePatch(oldBytes, newBytes, refStruct)
	if err != nil {
		return nil, fmt.Errorf("failed to create patch: %v", err)
	}

	return patchBytes, nil
}
