//go:build tools

package qlog

import (
	_ "k8s.io/api/core/v1"
	_ "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "k8s.io/cli-runtime/pkg/genericclioptions"
	_ "k8s.io/client-go/kubernetes"
)
