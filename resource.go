package main

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func IsSubresource(res metav1.APIResource) bool {
	parts := strings.Split(res.Name, "/")
	return len(parts) > 1
}
