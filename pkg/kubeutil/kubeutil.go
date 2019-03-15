package kubeutil

import (
	"encoding/json"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	"github.com/jzelinskie/cuddlefs/pkg/strutil"
)

// ContextName returns the name of cluster used in the current context.
func ContextName(kubeconfigPath string) (string, error) {
	cfg, err := clientcmd.LoadFromFile(kubeconfigPath)
	if err != nil {
		return "", err
	}
	return cfg.CurrentContext, nil
}

// Namespaces returns the list of unique namespaces in an unstructured list.
func Namespaces(ulist *unstructured.UnstructuredList) []string {
	namespaces := make([]string, 0, len(ulist.Items))
	for _, item := range ulist.Items {
		namespaces = append(namespaces, item.GetNamespace())
	}
	return strutil.Dedup(namespaces)
}

type Client struct {
	client.Client
	*discovery.DiscoveryClient
}

func NewClient(cfg *rest.Config) (*Client, error) {
	mapper, err := apiutil.NewDiscoveryRESTMapper(cfg)
	if err != nil {
		return nil, err
	}

	client, err := client.New(cfg, client.Options{Mapper: mapper})
	if err != nil {
		return nil, err
	}

	dclient, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, err
	}

	return &Client{client, dclient}, nil
}

func SplitGroupVersion(groupVersion string) (string, string) {
	parts := strings.Split(groupVersion, "/")
	var group, version string
	if len(parts) > 1 {
		group = parts[0]
	}
	if len(parts) > 1 {
		version = parts[1]
	} else if len(parts) > 0 {
		version = parts[0]
	} else {
		version = "v1"
	}
	return group, version
}

func UnstructuredToConfigMap(u *unstructured.Unstructured) (*corev1.ConfigMap, error) {
	data, err := json.Marshal(u)
	if err != nil {
		return nil, err
	}

	var configmap corev1.ConfigMap
	err = json.Unmarshal(data, &configmap)
	return &configmap, err
}
