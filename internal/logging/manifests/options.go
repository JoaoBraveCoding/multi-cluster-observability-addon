package manifests

import (
	loggingv1 "github.com/openshift/cluster-logging-operator/apis/logging/v1"
	"github.com/rhobs/multicluster-observability-addon/internal/addon"
	corev1 "k8s.io/api/core/v1"
)

type Options struct {
	Secrets             map[addon.Endpoint]corev1.Secret
	ClusterLogForwarder *loggingv1.ClusterLogForwarder
	Platform            addon.LogsOptions
	UserWorkloads       addon.LogsOptions
	SubscriptionChannel string
}
