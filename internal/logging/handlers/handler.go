package handlers

import (
	"context"

	loggingv1 "github.com/openshift/cluster-logging-operator/apis/logging/v1"
	"github.com/rhobs/multicluster-observability-addon/internal/addon"
	"github.com/rhobs/multicluster-observability-addon/internal/logging/manifests"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func BuildOptions(ctx context.Context, k8s client.Client, mcAddon *addonapiv1alpha1.ManagedClusterAddOn, adoc *addonapiv1alpha1.AddOnDeploymentConfig) (manifests.Options, error) {
	resources := manifests.Options{
		AddOnDeploymentConfig: adoc,
	}

	key := addon.GetObjectKey(mcAddon.Status.ConfigReferences, loggingv1.GroupVersion.Group, addon.ClusterLogForwardersResource)
	clf := &loggingv1.ClusterLogForwarder{}
	if err := k8s.Get(ctx, key, clf, &client.GetOptions{}); err != nil {
		return resources, err
	}
	resources.ClusterLogForwarder = clf

	targetSecretName := make(map[addon.Endpoint]string)
	for _, output := range clf.Spec.Outputs {
		targetSecretName[addon.Endpoint(output.Name)] = output.Secret.Name
	}

	targetSecrets, err := addon.GetSecrets(ctx, k8s, clf.Namespace, mcAddon.Namespace, targetSecretName)
	if err != nil {
		return resources, err
	}
	resources.Secrets = targetSecrets

	return resources, nil
}
