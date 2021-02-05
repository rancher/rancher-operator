package cluster

import (
	"context"
	"fmt"
	"strings"

	"github.com/rancher/norman/types/convert"
	v1 "github.com/rancher/rancher-operator/pkg/apis/rancher.cattle.io/v1"
	"github.com/rancher/rancher-operator/pkg/clients"
	mgmtcontrollers "github.com/rancher/rancher-operator/pkg/generated/controllers/management.cattle.io/v3"
	rocontrollers "github.com/rancher/rancher-operator/pkg/generated/controllers/rancher.cattle.io/v1"
	"github.com/rancher/rancher-operator/pkg/settings"
	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"github.com/rancher/wrangler/pkg/condition"
	appcontroller "github.com/rancher/wrangler/pkg/generated/controllers/apps/v1"
	corecontrollers "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/generic"
	"github.com/rancher/wrangler/pkg/kstatus"
	"github.com/rancher/wrangler/pkg/name"
	"github.com/rancher/wrangler/pkg/relatedresource"
	corev1 "k8s.io/api/core/v1"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const (
	byPrincipal = "by-principal"
	byAgentUser = "by-agent-user"
	byCluster   = "by-cluster"

	systemNamespace = "cattle-system"
)

type handler struct {
	rclusterCache     mgmtcontrollers.ClusterCache
	rclusters         mgmtcontrollers.ClusterClient
	deploymentCache   appcontroller.DeploymentCache
	daemonsetCache    appcontroller.DaemonSetCache
	clusterTokenCache mgmtcontrollers.ClusterRegistrationTokenCache
	clusterTokens     mgmtcontrollers.ClusterRegistrationTokenClient
	clusters          rocontrollers.ClusterController
	tokenCache        mgmtcontrollers.TokenCache
	userCache         mgmtcontrollers.UserCache
	secretCache       corecontrollers.SecretCache
	settings          mgmtcontrollers.SettingCache
}

func Register(
	ctx context.Context,
	clients *clients.Clients) {
	h := handler{
		rclusterCache:     clients.Management.Cluster().Cache(),
		rclusters:         clients.Management.Cluster(),
		daemonsetCache:    clients.Apps.DaemonSet().Cache(),
		deploymentCache:   clients.Apps.Deployment().Cache(),
		clusterTokenCache: clients.Management.ClusterRegistrationToken().Cache(),
		clusterTokens:     clients.Management.ClusterRegistrationToken(),
		clusters:          clients.Cluster(),
		tokenCache:        clients.Management.Token().Cache(),
		userCache:         clients.Management.User().Cache(),
		secretCache:       clients.Core.Secret().Cache(),
		settings:          clients.Management.Setting().Cache(),
	}

	clients.Cluster().OnChange(ctx, "cluster-update", h.onChange)
	rocontrollers.RegisterClusterGeneratingHandler(ctx,
		clients.Cluster(),
		clients.Apply.WithCacheTypes(clients.Management.Cluster(),
			clients.Core.Secret()),
		"Created",
		"cluster-create",
		h.generateCluster,
		&generic.GeneratingHandlerOptions{
			AllowClusterScoped: true,
		},
	)

	clusterCache := clients.Cluster().Cache()
	relatedresource.Watch(ctx, "cluster-watch", func(namespace, name string, obj runtime.Object) ([]relatedresource.Key, error) {
		cluster, ok := obj.(*v3.Cluster)
		if !ok {
			return nil, nil
		}
		operatorClusters, err := clusterCache.GetByIndex(byCluster, cluster.Name)
		if err != nil || len(operatorClusters) == 0 {
			// ignore
			return nil, nil
		}
		return []relatedresource.Key{
			{
				Namespace: operatorClusters[0].Namespace,
				Name:      operatorClusters[0].Name,
			},
		}, nil
	}, clients.Cluster(), clients.Management.Cluster())

	clusterCache.AddIndexer(byCluster, func(obj *v1.Cluster) ([]string, error) {
		if obj.Status.ClusterName == "" {
			return nil, nil
		}
		return []string{obj.Status.ClusterName}, nil
	})

	h.userCache.AddIndexer(byPrincipal, func(obj *v3.User) ([]string, error) {
		return obj.PrincipalIDs, nil
	})

	h.tokenCache.AddIndexer(byAgentUser, func(obj *v3.Token) ([]string, error) {
		// for rancher releases before 2.3 the label is not available, use name lookup too
		if obj.Labels["authn.management.cattle.io/kind"] != "agent" && !strings.HasPrefix(obj.Name, "agent-") {
			return nil, nil
		}
		return []string{obj.UserID}, nil
	})
}

func (h *handler) onChange(key string, cluster *v1.Cluster) (*v1.Cluster, error) {
	if cluster == nil {
		return cluster, nil
	}

	if cluster.Spec.ControlPlaneEndpoint == nil {
		// just set to something, this doesn't really make sense to me
		cluster = cluster.DeepCopy()
		cluster.Spec.ControlPlaneEndpoint = &v1.Endpoint{
			Host: "localhost",
			Port: 6443,
		}
		return h.clusters.Update(cluster)
	}
	return cluster, nil
}

func (h *handler) generateCluster(cluster *v1.Cluster, status v1.ClusterStatus) ([]runtime.Object, v1.ClusterStatus, error) {
	switch {
	case cluster.Spec.ImportedConfig != nil:
		return h.importCluster(cluster, status, v3.ClusterSpec{
			ImportedConfig: &v3.ImportedConfig{},
		})
	case cluster.Spec.ReferencedConfig != nil:
		return h.referenceCluster(cluster, status)
	case cluster.Spec.RancherKubernetesEngineConfig != nil:
		return h.createCluster(cluster, status, v3.ClusterSpec{
			ClusterSpecBase: v3.ClusterSpecBase{
				RancherKubernetesEngineConfig: cluster.Spec.RancherKubernetesEngineConfig,
				LocalClusterAuthEndpoint:      cluster.Spec.LocalClusterAuthEndpoint,
			},
		})
	case cluster.Spec.EKSConfig != nil:
		return h.createCluster(cluster, status, v3.ClusterSpec{
			EKSConfig: cluster.Spec.EKSConfig,
		})
	case cluster.Spec.K3SConfig != nil:
		return h.createCluster(cluster, status, v3.ClusterSpec{
			K3sConfig: cluster.Spec.K3SConfig,
		})
	case cluster.Spec.RKE2Config != nil:
		return h.createCluster(cluster, status, v3.ClusterSpec{
			Rke2Config: cluster.Spec.RKE2Config,
		})
	default:
		return nil, status, nil
	}
}

func (h *handler) createCluster(cluster *v1.Cluster, status v1.ClusterStatus, spec v3.ClusterSpec) ([]runtime.Object, v1.ClusterStatus, error) {
	spec.DisplayName = cluster.Name
	spec.Description = cluster.Annotations["field.cattle.io/description"]
	spec.FleetWorkspaceName = cluster.Namespace
	newCluster := &v3.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name.SafeConcatName("c", cluster.Namespace, cluster.Name),
			Labels:      cluster.Labels,
			Annotations: cluster.Annotations,
		},
		Spec: spec,
	}

	// We do this so that we don't clobber status because the rancher object is pretty dirty and doesn't have a status subresource
	data, err := convert.EncodeToMap(newCluster)
	if err != nil {
		return nil, status, err
	}
	data = map[string]interface{}{
		"metadata": data["metadata"],
		"spec":     data["spec"],
	}
	data["kind"] = "Cluster"
	data["apiVersion"] = "management.cattle.io/v3"

	return h.updateStatus([]runtime.Object{&unstructured.Unstructured{Object: data}}, cluster, status, newCluster)
}

func (h *handler) updateStatus(objs []runtime.Object, cluster *v1.Cluster, status v1.ClusterStatus, rCluster *v3.Cluster) ([]runtime.Object, v1.ClusterStatus, error) {
	ready := false
	existing, err := h.rclusterCache.Get(rCluster.Name)
	if err != nil && !apierror.IsNotFound(err) {
		return nil, status, err
	} else if err == nil {
		if condition.Cond("Ready").IsTrue(existing) {
			ready = true
		}
	}

	// Never set ready back to false because we will end up deleting the secret
	status.Ready = status.Ready || ready
	status.ObservedGeneration = cluster.Generation
	status.ClusterName = rCluster.Name
	if ready {
		kstatus.SetActive(&status)
	} else {
		kstatus.SetTransitioning(&status, "")
	}

	if status.Ready {
		secretName, secret, err := h.getKubeConfig(cluster, status)
		if err != nil {
			return nil, status, err
		}
		if secret != nil {
			objs = append(objs, secret)
		}
		status.ClientSecretName = secretName
	}

	return objs, status, nil
}

func (h *handler) getKubeConfig(cluster *v1.Cluster, status v1.ClusterStatus) (string, *corev1.Secret, error) {
	var (
		name       = cluster.Name + "-kubeconfig"
		tokenValue = ""
	)

	if cluster.Spec.ImportedConfig != nil && cluster.Spec.ImportedConfig.KubeConfigSecret == name {
		return name, nil, nil
	}

	users, err := h.userCache.GetByIndex(byPrincipal, fmt.Sprintf("system://%s", status.ClusterName))
	if err != nil {
		return "", nil, err
	}

	for _, user := range users {
		tokens, err := h.tokenCache.GetByIndex(byAgentUser, user.Name)
		if err != nil {
			return "", nil, err
		}
		for _, token := range tokens {
			tokenValue = fmt.Sprintf("%s:%s", token.Name, token.Token)
		}
	}

	if tokenValue == "" {
		return "", nil, fmt.Errorf("failed to find token for cluster %s", status.ClusterName)
	}

	serverURL, cacert, err := h.getServerURLAndCA()
	if err != nil {
		return "", nil, err
	}

	data, err := clientcmd.Write(clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			"cluster": {
				Server:                   fmt.Sprintf("%s/k8s/clusters/%s", serverURL, status.ClusterName),
				CertificateAuthorityData: []byte(strings.TrimSpace(cacert)),
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"user": {
				Token: tokenValue,
			},
		},
		Contexts: map[string]*clientcmdapi.Context{
			"default": {
				Cluster:  "cluster",
				AuthInfo: "user",
			},
		},
		CurrentContext: "default",
	})
	if err != nil {
		return "", nil, err
	}

	return name, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Namespace,
			Name:      name,
		},
		Data: map[string][]byte{
			"value": data,
		},
	}, nil
}

func (h *handler) getServerURLAndCA() (string, string, error) {
	serverURL, ca, err := settings.GetServerURLAndCA(h.settings)
	if err != nil {
		return "", "", err
	}

	tlsSecret, err := h.secretCache.Get(systemNamespace, "tls-rancher-internal-ca")
	if err != nil {
		return "", "", err
	}
	internalCA := string(tlsSecret.Data[corev1.TLSCertKey])

	if dp, err := h.deploymentCache.Get(systemNamespace, "rancher"); err == nil && dp.Spec.Replicas != nil && *dp.Spec.Replicas != 0 {
		return fmt.Sprintf("https://rancher.%s", systemNamespace), internalCA, nil
	}

	if _, err := h.daemonsetCache.Get(systemNamespace, "rancher"); err == nil {
		return fmt.Sprintf("https://rancher.%s", systemNamespace), internalCA, nil
	}

	return serverURL, ca, nil
}
