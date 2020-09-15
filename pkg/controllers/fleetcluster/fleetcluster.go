package fleetcluster

import (
	"context"

	fleet "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	v1 "github.com/rancher/rancher-operator/pkg/apis/rancher.cattle.io/v1"
	"github.com/rancher/rancher-operator/pkg/clients"
	mgmtcontrollers "github.com/rancher/rancher-operator/pkg/generated/controllers/management.cattle.io/v3"
	"github.com/rancher/rancher-operator/pkg/settings"
	mgmt "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"github.com/rancher/wrangler/pkg/generic"
	"github.com/rancher/wrangler/pkg/yaml"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
)

var (
	clusterName = "fleet.cattle.io/cluster-name"
)

type handler struct {
	settings mgmtcontrollers.SettingCache
	clusters mgmtcontrollers.ClusterClient
}

func Register(ctx context.Context, clients *clients.Clients) {
	h := &handler{
		settings: clients.Management.Setting().Cache(),
		clusters: clients.Management.Cluster(),
	}

	clients.Management.Cluster().OnChange(ctx, "fleet-cluster-label", h.addLabel)
	mgmtcontrollers.RegisterClusterGeneratingHandler(ctx,
		clients.Management.Cluster(),
		clients.Apply.
			WithCacheTypes(clients.Fleet.Cluster(),
				clients.Cluster()),
		"",
		"fleet-cluster",
		h.createCluster,
		nil,
	)
}

func (h *handler) addLabel(key string, cluster *mgmt.Cluster) (*mgmt.Cluster, error) {
	if cluster == nil {
		return cluster, nil
	}

	if cluster.Spec.Internal && cluster.Spec.FleetWorkspaceName == "" {
		cluster = cluster.DeepCopy()
		cluster.Spec.FleetWorkspaceName = "fleet-local"
		return h.clusters.Update(cluster)
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

		cluster = cluster.DeepCopy()
		cluster.Spec.FleetWorkspaceName = def
		cluster, err = h.clusters.Update(cluster)
		if err != nil {
			return nil, err
		}
	}

	if cluster.Spec.FleetWorkspaceName == "" {
		return cluster, nil
	}

	if cluster.Labels[clusterName] != cluster.Name {
		cluster = cluster.DeepCopy()
		if cluster.Labels == nil {
			cluster.Labels = map[string]string{}
		}
		cluster.Labels[clusterName] = cluster.Name
		return h.clusters.Update(cluster)
	}

	return cluster, nil
}

func (h *handler) createCluster(cluster *mgmt.Cluster, status mgmt.ClusterStatus) ([]runtime.Object, mgmt.ClusterStatus, error) {
	if cluster.Spec.FleetWorkspaceName == "" ||
		cluster.Labels[clusterName] == "" ||
		cluster.Spec.Internal {
		return nil, status, nil
	}

	if !mgmt.ClusterConditionReady.IsTrue(cluster) {
		return nil, status, generic.ErrSkip
	}

	labels := yaml.CleanAnnotationsForExport(cluster.Labels)
	labels["management.cattle.io/cluster-name"] = cluster.Name
	if errs := validation.IsValidLabelValue(cluster.Spec.DisplayName); len(errs) == 0 {
		labels["management.cattle.io/cluster-display-name"] = cluster.Spec.DisplayName
	}

	return []runtime.Object{
		&v1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cluster.Name,
				Namespace: cluster.Spec.FleetWorkspaceName,
				Labels:    labels,
			},
			Spec: v1.ClusterSpec{
				ReferencedConfig: &v1.ReferencedConfig{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							clusterName: cluster.Name,
						},
					},
				},
			},
		},
		&fleet.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cluster.Name,
				Namespace: cluster.Spec.FleetWorkspaceName,
				Labels:    labels,
			},
			Spec: fleet.ClusterSpec{
				KubeConfigSecret: cluster.Name + "-kubeconfig",
			},
		},
	}, status, nil
}
