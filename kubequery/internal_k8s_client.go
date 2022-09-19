/**
 * Copyright (c) 2020-present, The kubequery authors
 *
 * This source code is licensed as defined by the LICENSE file found in the
 * root directory of this source tree.
 *
 * SPDX-License-Identifier: (Apache-2.0 OR GPL-2.0-only)
 */

package k8s

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

var (
	lock        sync.Mutex
	clientset   kubernetes.Interface
	clusterUID  types.UID
	clusterName string
)

func UserConfig(token, addr string) *rest.Config{

	timeout := 10

	tlsConfig := rest.TLSClientConfig{Insecure: true}

	kubeConfig := &rest.Config{
		Host:            addr,
		BearerToken:     token,
		TLSClientConfig: tlsConfig,
		QPS:             250,
		Burst:           1000,
		Timeout:         time.Duration(timeout) * time.Second,
	}

	return kubeConfig
}


func initClientset(config *rest.Config) error {
	var err error
	if os.Getenv("KQ_TOKEN") != "" && os.Getenv("KQ_ADDR") != "" {
		config = UserConfig(os.Getenv("KQ_TOKEN"), os.Getenv("KQ_ADDR"))
	}

	if config == nil {
		conf, err := rest.InClusterConfig()
		if err != nil {
			if home := homedir.HomeDir(); home != "" {
				content, ferr := ioutil.ReadFile(filepath.Join(home, ".kube", "config"))
				if content != nil && ferr == nil {
					conf, err = clientcmd.RESTConfigFromKubeConfig(content)
					if err != nil {
						return err
					}
				}
			}
		}
		config = conf
	}

	// Suppress deprecation warnings
	config.WarningHandler = rest.NoWarnings{}

	clientset, err = kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}
	return nil
}

func initUID() error {
	ks, err := GetClient().CoreV1().Namespaces().Get(context.TODO(), "kube-system", v1.GetOptions{})
	if err != nil {
		return err
	}

	if os.Getenv("KQ_CLUSTER_NAME") != "" &&  os.Getenv("KQ_CLUSTER_UID") != ""{
		clusterName = os.Getenv("KQ_CLUSTER_NAME")
		clusterUID = types.UID(os.Getenv("KQ_CLUSTER_UID"))
		return nil
	}

	clusterUID = ks.UID
	if ks.ClusterName == "" {
		clusterName, err = os.Hostname()
		if err != nil {
			fmt.Println("Unable to determine hostname: ", err.Error())
		}
	} else {
		clusterName = ks.ClusterName
	}

	return nil
}

// Init creates in-cluster kubernetes configuration and a client set using the configuration.
// This returns error if KUBERNETES_SERVICE_HOST or KUBERNETES_SERVICE_PORT environment variables are not set.
func Init() error {
	lock.Lock()
	defer lock.Unlock()

	err := initClientset(nil)
	if err != nil {
		return err
	}
	err = initUID()
	if err != nil {
		return err
	}

	return nil
}

// GetClient returns kubernetes interface that can be used to communicate with API server.
func GetClient() kubernetes.Interface {
	return clientset
}

// GetClusterUID returns unique identifier for the current kubernetes cluster.
// This is same as the kube-system namespace UID.
func GetClusterUID() types.UID {
	return clusterUID
}

// GetClusterName returns cluster name provided by the kubernates API.
// If it is empty, it uses the pod hostname which should be set to the cluster name.
func GetClusterName() string {
	return clusterName
}

// SetClient is helper function to override the kubernetes interface with fake one for testing.
func SetClient(client kubernetes.Interface, uid types.UID, name string) {
	lock.Lock()
	defer lock.Unlock()

	clientset = client
	clusterUID = uid
	clusterName = name
}
