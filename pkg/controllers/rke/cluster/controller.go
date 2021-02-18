package cluster

import (
	"context"

	"github.com/rancher/lasso/pkg/dynamic"
	rancherv1 "github.com/rancher/rancher-operator/pkg/apis/rancher.cattle.io/v1"
	v1 "github.com/rancher/rancher-operator/pkg/apis/rke.cattle.io/v1"
	"github.com/rancher/rancher-operator/pkg/clients"
	rocontrollers "github.com/rancher/rancher-operator/pkg/generated/controllers/rancher.cattle.io/v1"
	clustercontrollers "github.com/rancher/rancher-operator/pkg/generated/controllers/rke.cattle.io/v1"
	corecontrollers "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/kstatus"
	"k8s.io/apimachinery/pkg/runtime"
)

type handler struct {
	dynamic       *dynamic.Controller
	clusterClient clustercontrollers.RKEClusterClient
	secretCache   corecontrollers.SecretCache
	secretClient  corecontrollers.SecretClient
}

func Register(ctx context.Context, clients *clients.Clients) {
	h := handler{
		dynamic:       clients.Dynamic,
		secretCache:   clients.Core.Secret().Cache(),
		secretClient:  clients.Core.Secret(),
		clusterClient: clients.RKE.RKECluster(),
	}

	clients.RKE.RKECluster().OnChange(ctx, "rke", h.UpdateSpec)

	clustercontrollers.RegisterRKEClusterStatusHandler(ctx,
		clients.RKE.RKECluster(),
		"",
		"rke-cluster",
		h.OnChange)

	rocontrollers.RegisterClusterGeneratingHandler(ctx,
		clients.Cluster.Cluster(),
		clients.Apply.
			WithSetID("rke-cluster").
			WithSetOwnerReference(false, false).
			WithDynamicLookup().
			WithCacheTypes(
				clients.CAPI.Cluster(),
				clients.CAPI.MachineDeployment(),
				clients.RKE.RKECluster(),
				clients.RKE.RKEBootstrapTemplate(),
			),
		"",
		"rke-cluster",
		h.OnRancherClusterChange,
		nil)
}

func (h *handler) UpdateSpec(key string, cluster *v1.RKECluster) (*v1.RKECluster, error) {
	if cluster == nil {
		return nil, nil
	}

	if cluster.Spec.ControlPlaneEndpoint == nil {
		cluster := cluster.DeepCopy()
		cluster.Spec.ControlPlaneEndpoint = &v1.Endpoint{
			Host: "localhost",
			Port: 6443,
		}
		return h.clusterClient.Update(cluster)
	}

	return cluster, nil
}

func (h *handler) OnChange(obj *v1.RKECluster, status v1.RKEClusterStatus) (v1.RKEClusterStatus, error) {
	status.Ready = true
	kstatus.SetActive(&status)
	return status, nil
}

func (h *handler) OnRancherClusterChange(obj *rancherv1.Cluster, status rancherv1.ClusterStatus) ([]runtime.Object, rancherv1.ClusterStatus, error) {
	if obj.Spec.RKEConfig == nil || obj.Status.ClusterName == "" {
		return nil, status, nil
	}
	objs, err := objects(obj, h.dynamic)
	return objs, status, err
}
