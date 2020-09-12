package cluster

import (
	"fmt"

	v1 "github.com/rancher/rancher-operator/pkg/apis/rancher.cattle.io/v1"
	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	claimedLabelNamespace = "rancher.cattle.io/claimed-by-namespace"
	claimedLabelName      = "rancher.cattle.io/claimed-by-name"
)

func (h *handler) referenceCluster(cluster *v1.Cluster, status v1.ClusterStatus) ([]runtime.Object, v1.ClusterStatus, error) {
	rCluster, err := h.claimCluster(cluster, status)
	if err != nil {
		return nil, status, err
	}

	return h.updateStatus(nil, cluster, status, rCluster)
}

func (h *handler) claimCluster(cluster *v1.Cluster, status v1.ClusterStatus) (*v3.Cluster, error) {
	if status.ClusterName != "" {
		return h.rclusterCache.Get(status.ClusterName)
	}

	if cluster.Spec.ReferencedConfig.Selector == nil {
		return nil, fmt.Errorf("missing selector for referenced cluster %s/%s", cluster.Namespace, cluster.Name)
	}

	claimed, err := h.rclusterCache.List(labels.SelectorFromSet(map[string]string{
		claimedLabelName:      cluster.Name,
		claimedLabelNamespace: cluster.Namespace,
	}))
	if err != nil {
		return nil, err
	}

	if len(claimed) > 1 {
		return nil, fmt.Errorf("more than one (%d) cluster is claimed by %s/%s remove %s and %s label on the undesired clusters",
			len(claimed), cluster.Namespace, cluster.Name, claimedLabelNamespace, claimedLabelName)
	}

	if len(claimed) == 1 {
		return claimed[0], nil
	}

	sel, err := metav1.LabelSelectorAsSelector(cluster.Spec.ReferencedConfig.Selector)
	if err != nil {
		return nil, err
	}

	available, err := h.rclusterCache.List(sel)
	if err != nil {
		return nil, err
	}

	for _, available := range available {
		if available.Labels[claimedLabelName] != "" || available.Labels[claimedLabelNamespace] != "" {
			continue
		}
		updated := available.DeepCopy()
		if updated.Labels == nil {
			updated.Labels = map[string]string{}
		}
		updated.Labels[claimedLabelName] = cluster.Name
		updated.Labels[claimedLabelNamespace] = cluster.Namespace
		return h.rclusters.Update(updated)
	}

	if len(available) == 0 {
		return nil, fmt.Errorf("failed to find a cluster that matches %s", cluster.Spec.ReferencedConfig.Selector)
	}

	for _, available := range available {
		if available.Labels[claimedLabelName] == "" || available.Labels[claimedLabelNamespace] == "" {
			continue
		}

		_, err := h.clusters.Cache().Get(available.Labels[claimedLabelNamespace], available.Labels[claimedLabelName])
		if apierror.IsNotFound(err) {
			copy := available.DeepCopy()
			delete(copy.Labels, claimedLabelNamespace)
			delete(copy.Labels, claimedLabelName)
			_, err := h.rclusters.Update(copy)
			if err != nil {
				return nil, err
			}
		}
	}

	return nil, fmt.Errorf("all clusters (%d) already claimed that match %s", len(available), cluster.Spec.ReferencedConfig.Selector)
}
