package kubeutil

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jzelinskie/cuddlefs/pkg/strutil"
)

func IsSubresource(res metav1.APIResource) bool {
	parts := strings.Split(res.Name, "/")
	return len(parts) > 1
}

func NamespaceNames(list *corev1.NamespaceList, err error) ([]string, error) {
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(list.Items))
	for _, ns := range list.Items {
		names = append(names, ns.Name)
	}

	return strutil.Dedup(names), nil
}

func PodNames(list *corev1.PodList, err error) ([]string, error) {
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(list.Items))
	for _, ns := range list.Items {
		names = append(names, ns.Name)
	}

	return strutil.Dedup(names), nil
}

func NamespacedResourceNames(serverResources []*metav1.APIResourceList, err error) ([]string, error) {
	if err != nil {
		return nil, err
	}

	names := make([]string, 0)
	for _, listing := range serverResources {
		for _, resource := range listing.APIResources {
			if resource.Namespaced && !IsSubresource(resource) {
				names = append(names, resource.Name)
			}
		}
	}

	return strutil.Dedup(names), nil
}

func ClusterResourceNames(serverResources []*metav1.APIResourceList, err error) ([]string, error) {
	if err != nil {
		return nil, err
	}

	names := make([]string, 0)
	for _, listing := range serverResources {
		for _, resource := range listing.APIResources {
			if !resource.Namespaced && !IsSubresource(resource) {
				names = append(names, resource.Name)
			}
		}
	}

	return strutil.Dedup(names), nil
}
