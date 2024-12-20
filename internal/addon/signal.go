package addon

import (
	"context"

	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Signal interface {
	// SupportedConfiguration returns true if the given configuration is a supported
	// use-case for the signal
	SupportedConfiguration(opts Options, cluster *clusterv1.ManagedCluster) bool
	// BuildOptions should build all the necessary configuration/resources to
	// generate the SignalValues that will be used to template the Helm chart of
	// the Signal
	BuildOptions(ctx context.Context, k8s client.Client, cluster *clusterv1.ManagedCluster, mcAddon *addonapiv1alpha1.ManagedClusterAddOn) (SignalOptions, error)
	// BuildValues should generate the SignalValues that will be used to template
	// the Helm chart of the Signal
	BuildValues(opts SignalOptions) (SignalValues, error)
}

type MCOA struct {
	Metrics Signal `json:"metrics"`
	Logs    Signal `json:"logs"`
}

type LogsValues struct {
	Enabled        bool   `json:"enabled"`
	CLFAnnotations string `json:"clfAnnotations"`
	CLFSpec        string `json:"clfSpec"`
}

