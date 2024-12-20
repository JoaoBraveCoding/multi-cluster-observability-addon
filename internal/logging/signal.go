package logging

import (
	"context"

	loggingv1 "github.com/openshift/cluster-logging-operator/api/observability/v1"
	"github.com/rhobs/multicluster-observability-addon/internal/addon"
	handlers "github.com/rhobs/multicluster-observability-addon/internal/logging/handlers"
	corev1 "k8s.io/api/core/v1"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type logs struct {
	Platform      addon.LogsOptions
	UserWorkloads addon.LogsOptions

	SubscriptionChannel string
	ClusterLogForwarder *loggingv1.ClusterLogForwarder
	Secrets             []corev1.Secret
	ConfigMaps          []corev1.ConfigMap
}

func NewLogs(opts addon.Options) *logs {
	return &logs{
		Platform:      opts.Platform.Logs,
		UserWorkloads: opts.UserWorkloads.Logs,
	}
}

func (l *logs) SupportedConfiguration(opts addon.Options, cluster *clusterv1.ManagedCluster) bool {
	if addon.IsHubCluster(cluster) {
		return false
	}

	if !opts.Platform.Logs.CollectionEnabled && !opts.UserWorkloads.Logs.CollectionEnabled {
		return false
	}

	return true
}

func (l *logs) BuildOptions(ctx context.Context, k8s client.Client, cluster *clusterv1.ManagedCluster, mcAddon *addonapiv1alpha1.ManagedClusterAddOn) (addon.SignalOptions, error) {
	loggingOpts, err := handlers.BuildOptions(ctx, k8s, mcAddon, l.Platform, l.UserWorkloads)
	if err != nil {
		return nil, err
	}
	return loggingOpts, nil
}

func (l *logs) BuildValues(opts addon.SignalOptions) (addon.SignalValues, error) {
	
}
