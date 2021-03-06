package fleetcluster

import (
	"context"
	"errors"

	fleet "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	v1 "github.com/rancher/rancher-operator/pkg/apis/rancher.cattle.io/v1"
	"github.com/rancher/rancher-operator/pkg/clients"
	clustercontroller "github.com/rancher/rancher-operator/pkg/controllers/cluster"
	mgmtcontrollers "github.com/rancher/rancher-operator/pkg/generated/controllers/management.cattle.io/v3"
	"github.com/rancher/rancher-operator/pkg/settings"
	mgmt "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"github.com/rancher/wrangler/pkg/apply"
	"github.com/rancher/wrangler/pkg/generic"
	"github.com/rancher/wrangler/pkg/relatedresource"
	"github.com/rancher/wrangler/pkg/yaml"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
)

type handler struct {
	settings mgmtcontrollers.SettingCache
	clusters mgmtcontrollers.ClusterClient
	apply    apply.Apply
}

func Register(ctx context.Context, clients *clients.Clients) {
	h := &handler{
		settings: clients.Management.Setting().Cache(),
		clusters: clients.Management.Cluster(),
		apply:    clients.Apply.WithCacheTypes(clients.Cluster.Cluster()),
	}

	clients.Management.Cluster().OnChange(ctx, "fleet-cluster-label", h.addLabel)
	mgmtcontrollers.RegisterClusterGeneratingHandler(ctx,
		clients.Management.Cluster(),
		clients.Apply.
			WithCacheTypes(clients.Fleet.Cluster(),
				clients.Cluster.Cluster()),
		"",
		"fleet-cluster",
		h.createCluster,
		nil,
	)

	relatedresource.WatchClusterScoped(ctx, "fleet-cluster-resolver", h.clusterToCluster,
		clients.Management.Cluster(), clients.Cluster.Cluster())
}

func (h *handler) clusterToCluster(namespace, name string, obj runtime.Object) ([]relatedresource.Key, error) {
	owner, err := h.apply.FindOwner(obj)
	if err != nil {
		// ignore error
		return nil, nil
	}
	if c, ok := owner.(*v1.Cluster); ok {
		return []relatedresource.Key{{
			Namespace: c.Namespace,
			Name:      c.Name,
		}}, nil
	}
	return nil, nil
}

func (h *handler) addLabel(key string, cluster *mgmt.Cluster) (*mgmt.Cluster, error) {
	if cluster == nil {
		return cluster, nil
	}

	if cluster.Spec.Internal && cluster.Spec.FleetWorkspaceName == "" {
		newCluster := cluster.DeepCopy()
		newCluster.Spec.FleetWorkspaceName = "fleet-local"
		return clustercontroller.PatchV3Cluster(h.clusters, cluster, newCluster)
	} else if cluster.Spec.Internal {
		return cluster, nil
	}

	if cluster.Spec.FleetWorkspaceName == "" {
		def, err := settings.Get(h.settings, "fleet-default-workspace-name")
		if err != nil {
			return nil, err
		}

		if def == "" {
			return cluster, nil
		}

		newCluster := cluster.DeepCopy()
		newCluster.Spec.FleetWorkspaceName = def
		cluster, err = clustercontroller.PatchV3Cluster(h.clusters, cluster, newCluster)
		if err != nil {
			return nil, err
		}
	}

	if cluster.Spec.FleetWorkspaceName == "" {
		return cluster, nil
	}

	return cluster, nil
}

func (h *handler) createCluster(mgmtCluster *mgmt.Cluster, status mgmt.ClusterStatus) ([]runtime.Object, mgmt.ClusterStatus, error) {
	if mgmtCluster.Spec.FleetWorkspaceName == "" ||
		mgmtCluster.Spec.Internal {
		return nil, status, nil
	}

	if !mgmt.ClusterConditionReady.IsTrue(mgmtCluster) {
		return nil, status, generic.ErrSkip
	}

	var (
		secretName       = mgmtCluster.Name + "-kubeconfig"
		fleetClusterName = mgmtCluster.Name
		rClusterName     = mgmtCluster.Name
		createCluster    = true
		objs             []runtime.Object
	)

	if owningCluster, err := h.apply.FindOwner(mgmtCluster); errors.Is(err, apply.ErrOwnerNotFound) || errors.Is(err, apply.ErrNoInformerFound) {
	} else if err != nil {
		return nil, status, err
	} else if rCluster, ok := owningCluster.(*v1.Cluster); ok {
		if rCluster.Status.ClientSecretName == "" {
			return nil, status, generic.ErrSkip
		}
		createCluster = false
		fleetClusterName = rCluster.Name
		rClusterName = rCluster.Name
		secretName = rCluster.Status.ClientSecretName
	}

	labels := yaml.CleanAnnotationsForExport(mgmtCluster.Labels)
	labels["management.cattle.io/cluster-name"] = mgmtCluster.Name
	labels["metadata.name"] = rClusterName
	if errs := validation.IsValidLabelValue(mgmtCluster.Spec.DisplayName); len(errs) == 0 {
		labels["management.cattle.io/cluster-display-name"] = mgmtCluster.Spec.DisplayName
	}

	if createCluster {
		objs = append(objs, &v1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rClusterName,
				Namespace: mgmtCluster.Spec.FleetWorkspaceName,
				Labels:    labels,
			},
			Spec: v1.ClusterSpec{
				ReferencedConfig: &v1.ReferencedConfig{
					ManagementClusterName: mgmtCluster.Name,
				},
			},
		})
	}

	objs = append(objs, &fleet.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fleetClusterName,
			Namespace: mgmtCluster.Spec.FleetWorkspaceName,
			Labels:    labels,
		},
		Spec: fleet.ClusterSpec{
			KubeConfigSecret: secretName,
		},
	})

	return objs, status, nil
}
