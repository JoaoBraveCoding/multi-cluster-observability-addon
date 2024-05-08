package handlers

import (
	"context"

	"github.com/ViaQ/logerr/v2/kverrors"
	loggingv1 "github.com/openshift/cluster-logging-operator/apis/logging/v1"
	"github.com/rhobs/multicluster-observability-addon/internal/addon"
	"github.com/rhobs/multicluster-observability-addon/internal/addon/authentication"
	"github.com/rhobs/multicluster-observability-addon/internal/logging/manifests"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func BuildOptions(k8s client.Client, cluster *clusterv1.ManagedCluster, mcAddon *addonapiv1alpha1.ManagedClusterAddOn, adoc *addonapiv1alpha1.AddOnDeploymentConfig) (manifests.Options, error) {
	resources := manifests.Options{
		AddOnDeploymentConfig: adoc,
	}

	clusterset := cluster.Labels["cluster.open-cluster-management.io/clusterset"]

	var clfList loggingv1.ClusterLogForwarderList
	if err := k8s.List(context.Background(), &clfList, &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{
			"cluster.open-cluster-management.io/clusterset": clusterset,
		}),
	}); err != nil {
		return resources, kverrors.Wrap(err, "failed to list clusterlogforwarders", "name", mcAddon.Namespace)
	}

	if len(clfList.Items) == 0 {
		klog.Info("default clusterlogforwarder will be used")
		labelSelector := metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      "cluster.open-cluster-management.io/clusterset",
					Operator: metav1.LabelSelectorOpDoesNotExist,
				},
			},
		}
		if err := k8s.List(context.Background(), &clfList, &client.ListOptions{
			LabelSelector: labels.SelectorFromSet(labelSelector.MatchLabels),
		}); err != nil {
			return resources, kverrors.Wrap(err, "failed to list default clusterlogforwarder", "name", mcAddon.Namespace)
		}
		if len(clfList.Items) == 0 {
			return resources, kverrors.New("failed to find a default clusterlogforwarder", "name", mcAddon.Namespace)
		}
	}
	resources.ClusterLogForwarder = &clfList.Items[0]

	authCM := &corev1.ConfigMap{}
	caCM := &corev1.ConfigMap{}
	for _, config := range mcAddon.Spec.Configs {
		switch config.ConfigGroupResource.Resource {
		case addon.ConfigMapResource:
			cm := &corev1.ConfigMap{}
			key := client.ObjectKey{Name: config.Name, Namespace: config.Namespace}
			if err := k8s.Get(context.Background(), key, cm, &client.GetOptions{}); err != nil {
				return resources, err
			}

			// Only care about cm's that configure logging
			if signal, ok := cm.Labels[addon.SignalLabelKey]; !ok || signal != addon.Logging.String() {
				continue
			}

			// If a cm has the ca annotation then it's the configmap containing the ca
			if _, ok := cm.Annotations[authentication.AnnotationCAToInject]; ok {
				caCM = cm
				continue
			}

			// If a cm doesn't have a target label then it's configuring authentication
			if _, ok := cm.Annotations[manifests.AnnotationTargetOutputName]; !ok {
				authCM = cm
				continue
			}

			resources.ConfigMaps = append(resources.ConfigMaps, *cm)
		}
	}

	ctx := context.Background()
	authConfig := manifests.AuthDefaultConfig
	authConfig.MTLSConfig.CommonName = mcAddon.Namespace
	if len(caCM.Data) > 0 {
		if ca, ok := caCM.Data["service-ca.crt"]; ok {
			authConfig.MTLSConfig.CAToInject = ca
		} else {
			return resources, kverrors.New("missing ca bundle in configmap", "key", "service-ca.crt")
		}
	}

	secretsProvider, err := authentication.NewSecretsProvider(k8s, mcAddon.Namespace, addon.Logging, authConfig)
	if err != nil {
		return resources, err
	}

	targetsSecret, err := secretsProvider.GenerateSecrets(ctx, authentication.BuildAuthenticationMap(authCM.Data))
	if err != nil {
		return resources, err
	}

	resources.Secrets, err = secretsProvider.FetchSecrets(ctx, targetsSecret, manifests.AnnotationTargetOutputName)
	if err != nil {
		return resources, err
	}

	return resources, nil
}
