package logging

import (
	"context"
	"testing"

	loggingv1 "github.com/openshift/cluster-logging-operator/api/observability/v1"
	"github.com/rhobs/multicluster-observability-addon/internal/addon"
	"github.com/rhobs/multicluster-observability-addon/internal/logging/handlers"
	"github.com/rhobs/multicluster-observability-addon/internal/logging/manifests"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	lokiv1 "github.com/grafana/loki/operator/api/loki/v1"
	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	"open-cluster-management.io/addon-framework/pkg/addonfactory"
	"open-cluster-management.io/addon-framework/pkg/addonmanager/addontesting"
	"open-cluster-management.io/addon-framework/pkg/agent"
	addonutils "open-cluster-management.io/addon-framework/pkg/utils"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	fakeaddon "open-cluster-management.io/api/client/addon/clientset/versioned/fake"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	_ = loggingv1.AddToScheme(scheme.Scheme)
	_ = loggingv1.AddToScheme(scheme.Scheme)
	_ = operatorsv1.AddToScheme(scheme.Scheme)
	_ = operatorsv1alpha1.AddToScheme(scheme.Scheme)
	_ = addonapiv1alpha1.AddToScheme(scheme.Scheme)
	_ = certmanagerv1.AddToScheme(scheme.Scheme)
	_ = lokiv1.AddToScheme(scheme.Scheme)
)

func fakeGetValues(k8s client.Client, isHub bool) addonfactory.GetValuesFunc {
	return func(
		_ *clusterv1.ManagedCluster,
		mcAddon *addonapiv1alpha1.ManagedClusterAddOn,
	) (addonfactory.Values, error) {
		aodc := &addonapiv1alpha1.AddOnDeploymentConfig{}
		keys := addon.GetObjectKeys(mcAddon.Status.ConfigReferences, addonutils.AddOnDeploymentConfigGVR.Group, addon.AddonDeploymentConfigResource)
		if err := k8s.Get(context.TODO(), keys[0], aodc, &client.GetOptions{}); err != nil {
			return nil, err
		}
		addonOpts, err := addon.BuildOptions(aodc)
		if err != nil {
			return nil, err
		}

		opts, err := handlers.BuildOptions(context.TODO(), k8s, mcAddon, addonOpts.Platform.Logs, addonOpts.UserWorkloads.Logs, isHub, "myhub.foo.com")
		if err != nil {
			return nil, err
		}

		logging, err := manifests.BuildValues(opts)
		if err != nil {
			return nil, err
		}

		return addonfactory.JsonStructToValues(logging)
	}
}

func Test_Logging_Unmanaged_Collection(t *testing.T) {
	var (
		// Addon envinronment and registration
		managedCluster      *clusterv1.ManagedCluster
		managedClusterAddOn *addonapiv1alpha1.ManagedClusterAddOn

		// Addon configuration
		addOnDeploymentConfig *addonapiv1alpha1.AddOnDeploymentConfig
		clf                   *loggingv1.ClusterLogForwarder
		staticCred            *corev1.Secret
		caConfigMap           *corev1.ConfigMap

		// Test clients
		fakeKubeClient  client.Client
		fakeAddonClient *fakeaddon.Clientset
	)

	// Setup a managed cluster
	managedCluster = addontesting.NewManagedCluster("cluster-1")

	// Register the addon for the managed cluster
	managedClusterAddOn = addontesting.NewAddon("test", "cluster-1")
	managedClusterAddOn.Status.ConfigReferences = []addonapiv1alpha1.ConfigReference{
		{
			ConfigGroupResource: addonapiv1alpha1.ConfigGroupResource{
				Group:    "addon.open-cluster-management.io",
				Resource: "addondeploymentconfigs",
			},
			ConfigReferent: addonapiv1alpha1.ConfigReferent{
				Namespace: "open-cluster-management-observability",
				Name:      "multicluster-observability-addon",
			},
			DesiredConfig: &addonapiv1alpha1.ConfigSpecHash{
				ConfigReferent: addonapiv1alpha1.ConfigReferent{
					Namespace: "open-cluster-management-observability",
					Name:      "multicluster-observability-addon",
				},
				SpecHash: "fake-spec-hash",
			},
		},
		{
			ConfigGroupResource: addonapiv1alpha1.ConfigGroupResource{
				Group:    "observability.openshift.io",
				Resource: "clusterlogforwarders",
			},
			ConfigReferent: addonapiv1alpha1.ConfigReferent{
				Namespace: "open-cluster-management-observability",
				Name:      "mcoa-instance",
			},
		},
	}

	// Setup configuration resources: ClusterLogForwarder, AddOnDeploymentConfig, Secrets, ConfigMaps
	clf = &loggingv1.ClusterLogForwarder{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mcoa-instance",
			Namespace: "open-cluster-management-observability",
		},
		Spec: loggingv1.ClusterLogForwarderSpec{
			ServiceAccount: loggingv1.ServiceAccount{
				Name: "mcoa-sa",
			},
			Inputs: []loggingv1.InputSpec{
				{
					Name: "app-logs",
					Application: &loggingv1.Application{
						Includes: []loggingv1.NamespaceContainerSpec{
							{
								Namespace: "ns-1",
							},
							{
								Namespace: "ns-2",
							},
						},
					},
				},
				{
					Name:           "infra-logs",
					Infrastructure: &loggingv1.Infrastructure{},
				},
			},
			Outputs: []loggingv1.OutputSpec{
				{
					Name: "app-logs",
					Type: loggingv1.OutputTypeLoki,
					Loki: &loggingv1.Loki{
						Authentication: &loggingv1.HTTPAuthentication{
							Token: &loggingv1.BearerToken{
								From: loggingv1.BearerTokenFromSecret,
								Secret: &loggingv1.BearerTokenSecretKey{
									Name: "static-authentication",
									Key:  "pass",
								},
							},
						},
						LabelKeys: []string{"key-1", "key-2"},
						TenantKey: "tenant-x",
					},
					// Simply here to test the ConfigMap reference
					TLS: &loggingv1.OutputTLSSpec{
						TLSSpec: loggingv1.TLSSpec{
							CA: &loggingv1.ValueReference{
								ConfigMapName: "foo",
							},
						},
					},
				},
				{
					Name: "cluster-logs",
					Type: loggingv1.OutputTypeCloudwatch,
					Cloudwatch: &loggingv1.Cloudwatch{
						Authentication: &loggingv1.CloudwatchAuthentication{
							Type: loggingv1.CloudwatchAuthTypeAccessKey,
							AWSAccessKey: &loggingv1.CloudwatchAWSAccessKey{
								KeyId: loggingv1.SecretReference{
									SecretName: "static-authentication",
									Key:        "key",
								},
								KeySecret: loggingv1.SecretReference{
									SecretName: "static-authentication",
									Key:        "pass",
								},
							},
						},
					},
				},
			},
			Pipelines: []loggingv1.PipelineSpec{
				{
					Name:       "app-logs",
					InputRefs:  []string{"app-logs"},
					OutputRefs: []string{"app-logs"},
				},
				{
					Name:       "cluster-logs",
					InputRefs:  []string{"infra-logs", string(loggingv1.InputTypeAudit)},
					OutputRefs: []string{"cluster-logs"},
				},
			},
		},
	}

	staticCred = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "static-authentication",
			Namespace: "open-cluster-management-observability",
		},
		Data: map[string][]byte{
			"key":  []byte("data"),
			"pass": []byte("data"),
		},
	}

	caConfigMap = &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "open-cluster-management-observability",
		},
		Data: map[string]string{
			"foo": "bar",
		},
	}

	addOnDeploymentConfig = &addonapiv1alpha1.AddOnDeploymentConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "multicluster-observability-addon",
			Namespace: "open-cluster-management-observability",
		},
		Spec: addonapiv1alpha1.AddOnDeploymentConfigSpec{
			CustomizedVariables: []addonapiv1alpha1.CustomizedVariable{
				{
					Name:  "loggingSubscriptionChannel",
					Value: "stable-6.0",
				},
				{
					Name:  "platformLogsCollection",
					Value: "clusterlogforwarders.v1.observability.openshift.io",
				},
			},
		},
	}

	// Setup the fake k8s client
	fakeKubeClient = fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(clf, staticCred, caConfigMap, addOnDeploymentConfig).
		Build()

	// Setup the fake addon client
	fakeAddonClient = fakeaddon.NewSimpleClientset(addOnDeploymentConfig)
	addonConfigValuesFn := addonfactory.GetAddOnDeploymentConfigValues(
		addonfactory.NewAddOnDeploymentConfigGetter(fakeAddonClient),
		addonfactory.ToAddOnCustomizedVariableValues,
	)

	// Wire everything together to a fake addon instance
	loggingAgentAddon, err := addonfactory.NewAgentAddonFactory(addon.Name, addon.FS, addon.LoggingChartDir).
		WithGetValuesFuncs(addonConfigValuesFn, fakeGetValues(fakeKubeClient, false)).
		WithAgentRegistrationOption(&agent.RegistrationOption{}).
		WithScheme(scheme.Scheme).
		BuildHelmAgentAddon()
	if err != nil {
		klog.Fatalf("failed to build agent %v", err)
	}

	// Render manifests and return them as k8s runtime objects
	objects, err := loggingAgentAddon.Manifests(managedCluster, managedClusterAddOn)
	require.NoError(t, err)
	require.Equal(t, 11, len(objects))

	for _, obj := range objects {
		switch obj := obj.(type) {
		case *corev1.ServiceAccount:
			require.Equal(t, "mcoa-sa", obj.Name)
		case *operatorsv1alpha1.Subscription:
			require.Equal(t, obj.Spec.Channel, "stable-6.0")
		case *loggingv1.ClusterLogForwarder:
			require.NotNil(t, obj.Spec.Outputs[0].Loki.Authentication.Token.Secret)
			require.NotNil(t, obj.Spec.Outputs[1].Cloudwatch.Authentication.AWSAccessKey)
			require.Equal(t, "static-authentication", obj.Spec.Outputs[0].Loki.Authentication.Token.Secret.Name)
			require.Equal(t, "static-authentication", obj.Spec.Outputs[1].Cloudwatch.Authentication.AWSAccessKey.KeySecret.SecretName)
			// Check name and namespace to make sure that if we change the helm
			// manifests that we don't break the addon probes
			require.Equal(t, addon.SpokeUnmanagedCLFName, obj.Name)
			require.Equal(t, addon.LoggingNamespace, obj.Namespace)
		case *corev1.Secret:
			if obj.Name == "static-authentication" {
				require.Equal(t, staticCred.Data, obj.Data)
			}
		case *corev1.ConfigMap:
			if obj.Name == "foo" {
				require.Equal(t, caConfigMap.Data, obj.Data)
			}
		}
	}
}

func Test_Logging_Managed_Collection(t *testing.T) {
	var (
		// Addon envinronment and registration
		managedCluster      *clusterv1.ManagedCluster
		managedClusterAddOn *addonapiv1alpha1.ManagedClusterAddOn

		// Addon configuration
		addOnDeploymentConfig *addonapiv1alpha1.AddOnDeploymentConfig

		// Test clients
		fakeKubeClient  client.Client
		fakeAddonClient *fakeaddon.Clientset
	)

	// Setup a managed cluster
	managedCluster = addontesting.NewManagedCluster("cluster-1")

	// Register the addon for the managed cluster
	managedClusterAddOn = addontesting.NewAddon("test", "cluster-1")
	managedClusterAddOn.Status.ConfigReferences = []addonapiv1alpha1.ConfigReference{
		{
			ConfigGroupResource: addonapiv1alpha1.ConfigGroupResource{
				Group:    "addon.open-cluster-management.io",
				Resource: "addondeploymentconfigs",
			},
			ConfigReferent: addonapiv1alpha1.ConfigReferent{
				Namespace: "open-cluster-management-observability",
				Name:      "multicluster-observability-addon",
			},
			DesiredConfig: &addonapiv1alpha1.ConfigSpecHash{
				ConfigReferent: addonapiv1alpha1.ConfigReferent{
					Namespace: "open-cluster-management-observability",
					Name:      "multicluster-observability-addon",
				},
				SpecHash: "fake-spec-hash",
			},
		},
	}

	addOnDeploymentConfig = &addonapiv1alpha1.AddOnDeploymentConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "multicluster-observability-addon",
			Namespace: "open-cluster-management-observability",
		},
		Spec: addonapiv1alpha1.AddOnDeploymentConfigSpec{
			CustomizedVariables: []addonapiv1alpha1.CustomizedVariable{
				{
					Name:  "loggingSubscriptionChannel",
					Value: "stable-6.0",
				},
				{
					Name:  "platformLogsStorage",
					Value: "lokistacks.v1.loki.grafana.io",
				},
			},
		},
	}

	// Emulate the secret that would've been created by the cert-manager
	mtls := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mcoa-logging-managed-collection-tls",
			Namespace: "cluster-1",
		},
		Data: map[string][]byte{
			"ca.crt":  []byte("data"),
			"tls.crt": []byte("data"),
			"tls.key": []byte("data"),
		},
	}

	expectedCLF := &loggingv1.ClusterLogForwarder{
		TypeMeta: metav1.TypeMeta{
			APIVersion: loggingv1.GroupVersion.String(),
			Kind:       "ClusterLogForwarder",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mcoa-managed-instance",
			Namespace: "openshift-logging",
			Labels: map[string]string{
				"release": "multicluster-observability-addon",
				"chart":   "collection-1.0.0",
				"app":     "collection",
			},
		},
		Spec: loggingv1.ClusterLogForwarderSpec{
			ServiceAccount: loggingv1.ServiceAccount{
				Name: "mcoa-logging-managed-collector",
			},
			Outputs: []loggingv1.OutputSpec{
				{
					Name: "hub-lokistack",
					Type: loggingv1.OutputTypeOTLP,
					OTLP: &loggingv1.OTLP{
						URL: "https://lokistack-hub-openshift-logging.myhub.foo.com/api/logs/v1/cluster-1",
					},
					TLS: &loggingv1.OutputTLSSpec{
						TLSSpec: loggingv1.TLSSpec{
							CA: &loggingv1.ValueReference{
								Key:        "ca.crt",
								SecretName: "mcoa-managed-collector-tls",
							},
							Certificate: &loggingv1.ValueReference{
								Key:        "tls.crt",
								SecretName: "mcoa-managed-collector-tls",
							},
							Key: &loggingv1.SecretReference{
								Key:        "tls.key",
								SecretName: "mcoa-managed-collector-tls",
							},
						},
					},
				},
			},
			Pipelines: []loggingv1.PipelineSpec{
				{
					Name:       "infra-hub-lokistack",
					InputRefs:  []string{"infrastructure"},
					OutputRefs: []string{"hub-lokistack"},
				},
			},
		},
	}

	// Setup the fake k8s client
	fakeKubeClient = fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(addOnDeploymentConfig, mtls).
		Build()

	// Setup the fake addon client
	fakeAddonClient = fakeaddon.NewSimpleClientset(addOnDeploymentConfig)
	addonConfigValuesFn := addonfactory.GetAddOnDeploymentConfigValues(
		addonfactory.NewAddOnDeploymentConfigGetter(fakeAddonClient),
		addonfactory.ToAddOnCustomizedVariableValues,
	)

	// Wire everything together to a fake addon instance
	loggingAgentAddon, err := addonfactory.NewAgentAddonFactory(addon.Name, addon.FS, addon.LoggingChartDir).
		WithGetValuesFuncs(addonConfigValuesFn, fakeGetValues(fakeKubeClient, false)).
		WithAgentRegistrationOption(&agent.RegistrationOption{}).
		WithScheme(scheme.Scheme).
		BuildHelmAgentAddon()
	if err != nil {
		klog.Fatalf("failed to build agent %v", err)
	}

	// Render manifests and return them as k8s runtime objects
	objects, err := loggingAgentAddon.Manifests(managedCluster, managedClusterAddOn)
	require.NoError(t, err)
	require.Equal(t, 9, len(objects))

	for _, obj := range objects {
		switch obj := obj.(type) {
		case *corev1.ServiceAccount:
			require.Equal(t, "mcoa-logging-managed-collector", obj.Name)
		case *operatorsv1alpha1.Subscription:
			require.Equal(t, obj.Spec.Channel, "stable-6.0")
		case *loggingv1.ClusterLogForwarder:
			require.Equal(t, expectedCLF, obj)
			// Check name and namespace to make sure that if we change the helm
			// manifests that we don't break the addon probes
			require.Equal(t, addon.SpokeManagedCLFName, obj.Name)
			require.Equal(t, addon.LoggingNamespace, obj.Namespace)
		}
	}
}

func Test_Logging_Managed_Storage(t *testing.T) {
	var (
		// Addon envinronment and registration
		managedCluster      *clusterv1.ManagedCluster
		managedClusterAddOn *addonapiv1alpha1.ManagedClusterAddOn

		// Addon configuration
		addOnDeploymentConfig *addonapiv1alpha1.AddOnDeploymentConfig

		// Test clients
		fakeKubeClient  client.Client
		fakeAddonClient *fakeaddon.Clientset
	)

	// Setup a managed cluster
	managedCluster = addontesting.NewManagedCluster("cluster-1")

	// Register the addon for the managed cluster
	managedClusterAddOn = addontesting.NewAddon("test", "cluster-1")
	managedClusterAddOn.Status.ConfigReferences = []addonapiv1alpha1.ConfigReference{
		{
			ConfigGroupResource: addonapiv1alpha1.ConfigGroupResource{
				Group:    "addon.open-cluster-management.io",
				Resource: "addondeploymentconfigs",
			},
			ConfigReferent: addonapiv1alpha1.ConfigReferent{
				Namespace: "open-cluster-management-observability",
				Name:      "multicluster-observability-addon",
			},
			DesiredConfig: &addonapiv1alpha1.ConfigSpecHash{
				ConfigReferent: addonapiv1alpha1.ConfigReferent{
					Namespace: "open-cluster-management-observability",
					Name:      "multicluster-observability-addon",
				},
				SpecHash: "fake-spec-hash",
			},
		},
	}

	addOnDeploymentConfig = &addonapiv1alpha1.AddOnDeploymentConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "multicluster-observability-addon",
			Namespace: "open-cluster-management-observability",
		},
		Spec: addonapiv1alpha1.AddOnDeploymentConfigSpec{
			CustomizedVariables: []addonapiv1alpha1.CustomizedVariable{
				{
					Name:  "loggingSubscriptionChannel",
					Value: "stable-6.0",
				},
				{
					Name:  "platformLogsStorage",
					Value: "lokistacks.v1.loki.grafana.io",
				},
			},
		},
	}

	// Emulate the secret that would've been created by the cert-manager
	mtls := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mcoa-logging-managed-storage-tls",
			Namespace: "openshift-logging",
		},
		Data: map[string][]byte{
			"ca.crt":  []byte("data"),
			"tls.crt": []byte("data"),
			"tls.key": []byte("data"),
		},
	}

	objstorage := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mcoa-logging-managed-storage-objstorage",
			Namespace: "openshift-logging",
		},
		Data: map[string][]byte{
			"foo": []byte("bar"),
		},
	}

	expectedLS := lokiv1.LokiStack{
		TypeMeta: metav1.TypeMeta{
			APIVersion: lokiv1.GroupVersion.String(),
			Kind:       "LokiStack",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mcoa-managed-storage",
			Namespace: "openshift-logging",
			Labels: map[string]string{
				"release": "multicluster-observability-addon",
				"chart":   "storage-1.0.0",
				"app":     "storage",
			},
		},
		Spec: lokiv1.LokiStackSpec{
			Size:             lokiv1.SizeOneXDemo,
			StorageClassName: "gp3-csi",
			Storage: lokiv1.ObjectStorageSpec{
				Secret: lokiv1.ObjectStorageSecretSpec{
					Type: "s3",
					Name: "mcoa-logging-managed-storage",
				},
				Schemas: []lokiv1.ObjectStorageSchema{
					{
						Version:       lokiv1.ObjectStorageSchemaV13,
						EffectiveDate: "2024-11-18",
					},
				},
			},
			Tenants: &lokiv1.TenantsSpec{
				Mode: lokiv1.Static,
				Authentication: []lokiv1.AuthenticationSpec{
					{
						TenantName: "tenant-1",
						TenantID:   "tenant-1",
						MTLS: &lokiv1.MTLSSpec{
							CA: &lokiv1.CASpec{
								CAKey: "ca.crt",
								CA:    "mcoa-managed-storage-tls",
							},
						},
					},
					{
						TenantName: "tenant-2",
						TenantID:   "tenant-2",
						MTLS: &lokiv1.MTLSSpec{
							CA: &lokiv1.CASpec{
								CAKey: "ca.crt",
								CA:    "mcoa-managed-storage-tls",
							},
						},
					},
				},
				Authorization: &lokiv1.AuthorizationSpec{
					Roles: []lokiv1.RoleSpec{
						{
							Name:        "tenant-1-logs",
							Resources:   []string{"logs"},
							Permissions: []lokiv1.PermissionType{"read", "write"},
							Tenants:     []string{"tenant-1"},
						},
						{
							Name:        "tenant-2-logs",
							Resources:   []string{"logs"},
							Permissions: []lokiv1.PermissionType{"read", "write"},
							Tenants:     []string{"tenant-2"},
						},
						{
							Name:        "cluster-reader",
							Resources:   []string{"logs"},
							Permissions: []lokiv1.PermissionType{"read"},
							Tenants:     []string{"tenant-1", "tenant-2"},
						},
					},
					RoleBindings: []lokiv1.RoleBindingsSpec{
						{
							Name:  "tenant-1-logs",
							Roles: []string{"tenant-1-logs"},
							Subjects: []lokiv1.Subject{
								{
									Kind: "Group",
									Name: "tenant-1",
								},
							},
						},
						{
							Name:  "tenant-2-logs",
							Roles: []string{"tenant-2-logs"},
							Subjects: []lokiv1.Subject{
								{
									Kind: "Group",
									Name: "tenant-2",
								},
							},
						},
						{
							Name:  "cluster-reader",
							Roles: []string{"cluster-reader"},
							Subjects: []lokiv1.Subject{
								{
									Kind: "Group",
									Name: "mcoa-logs-admin",
								},
							},
						},
					},
				},
			},
		},
	}

	// Setup the fake k8s client
	fakeKubeClient = fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(addOnDeploymentConfig, mtls, objstorage).
		Build()

	// Setup the fake addon client
	fakeAddonClient = fakeaddon.NewSimpleClientset(addOnDeploymentConfig)
	addonConfigValuesFn := addonfactory.GetAddOnDeploymentConfigValues(
		addonfactory.NewAddOnDeploymentConfigGetter(fakeAddonClient),
		addonfactory.ToAddOnCustomizedVariableValues,
	)

	// Wire everything together to a fake addon instance
	loggingAgentAddon, err := addonfactory.NewAgentAddonFactory(addon.Name, addon.FS, addon.LoggingChartDir).
		WithGetValuesFuncs(addonConfigValuesFn, fakeGetValues(fakeKubeClient, true)).
		WithAgentRegistrationOption(&agent.RegistrationOption{}).
		WithScheme(scheme.Scheme).
		BuildHelmAgentAddon()
	if err != nil {
		klog.Fatalf("failed to build agent %v", err)
	}

	// Render manifests and return them as k8s runtime objects
	objects, err := loggingAgentAddon.Manifests(managedCluster, managedClusterAddOn)
	require.NoError(t, err)
	require.Equal(t, 6, len(objects))

	for _, obj := range objects {
		switch obj := obj.(type) {
		case *corev1.ServiceAccount:
			require.Equal(t, "mcoa-logging-managed-collector", obj.Name)
		case *operatorsv1alpha1.Subscription:
			require.Equal(t, obj.Spec.Channel, "stable-6.0")
		case *lokiv1.LokiStack:
			require.Equal(t, expectedLS, obj)
			// Check name and namespace to make sure that if we change the helm
			// manifests that we don't break the addon probes
			require.Equal(t, addon.SpokeManagedCLFName, obj.Name)
			require.Equal(t, addon.LoggingNamespace, obj.Namespace)
		}
	}
}
