package helm

import (
	"context"

	"github.com/rhobs/multicluster-observability-addon/internal/addon"
	lhandlers "github.com/rhobs/multicluster-observability-addon/internal/logging/handlers"
	lmanifests "github.com/rhobs/multicluster-observability-addon/internal/logging/manifests"
	thandlers "github.com/rhobs/multicluster-observability-addon/internal/tracing/handlers"
	tmanifests "github.com/rhobs/multicluster-observability-addon/internal/tracing/manifests"
	"open-cluster-management.io/addon-framework/pkg/addonfactory"
	addonutils "open-cluster-management.io/addon-framework/pkg/utils"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const annotationLocalCluster = "local-cluster"

type HelmChartValues struct {
	Enabled bool                     `json:"enabled"`
	Logging lmanifests.LoggingValues `json:"logging"`
	Tracing tmanifests.TracingValues `json:"tracing"`
}

func GetValuesFunc(ctx context.Context, k8s client.Client) addonfactory.GetValuesFunc {
	return func(
		cluster *clusterv1.ManagedCluster,
		mcAddon *addonapiv1alpha1.ManagedClusterAddOn,
	) (addonfactory.Values, error) {
		// if hub cluster, then don't install anything
		if isHubCluster(cluster) {
			return addonfactory.JsonStructToValues(HelmChartValues{})
		}

		aodc, err := getAddOnDeploymentConfig(ctx, k8s, mcAddon)
		if err != nil {
			return nil, err
		}
		opts, err := addon.BuildOptions(aodc)
		if err != nil {
			return nil, err
		}

		if !opts.Platform.Enabled && !opts.UserWorkloads.Enabled {
			return addonfactory.JsonStructToValues(HelmChartValues{})
		}

		userValues := HelmChartValues{
			Enabled: true,
		}

		if opts.Platform.Logs.CollectionEnabled || opts.UserWorkloads.Logs.CollectionEnabled {
			loggingOpts, err := lhandlers.BuildOptions(ctx, k8s, mcAddon, opts.Platform.Logs, opts.Platform.Logs)
			if err != nil {
				return nil, err
			}

			logging, err := lmanifests.BuildValues(loggingOpts)
			if err != nil {
				return nil, err
			}
			userValues.Logging = *logging
		}

		if opts.UserWorkloads.Traces.CollectionEnabled {
			tracingOpts, err := thandlers.BuildOptions(ctx, k8s, mcAddon, opts.UserWorkloads.Traces)
			if err != nil {
				return nil, err
			}

			tracing, err := tmanifests.BuildValues(tracingOpts)
			if err != nil {
				return nil, err
			}
			userValues.Tracing = tracing
		}

		return addonfactory.JsonStructToValues(userValues)
	}
}

func getAddOnDeploymentConfig(ctx context.Context, k8s client.Client, mcAddon *addonapiv1alpha1.ManagedClusterAddOn) (*addonapiv1alpha1.AddOnDeploymentConfig, error) {
	key := addon.GetObjectKey(mcAddon.Status.ConfigReferences, addonutils.AddOnDeploymentConfigGVR.Group, addon.AddonDeploymentConfigResource)
	addOnDeployment := &addonapiv1alpha1.AddOnDeploymentConfig{}
	if err := k8s.Get(ctx, key, addOnDeployment, &client.GetOptions{}); err != nil {
		// TODO(JoaoBraveCoding) Add proper error handling
		return addOnDeployment, err
	}
	return addOnDeployment, nil
}

func isHubCluster(cluster *clusterv1.ManagedCluster) bool {
	val, ok := cluster.Labels[annotationLocalCluster]
	if !ok {
		return false
	}
	return val == "true"
}
