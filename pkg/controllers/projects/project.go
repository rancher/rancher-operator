package projects

import (
	"context"

	v1 "github.com/rancher/rancher-operator/pkg/apis/rancher.cattle.io/v1"
	"github.com/rancher/rancher-operator/pkg/clients"
	rocontrollers "github.com/rancher/rancher-operator/pkg/generated/controllers/rancher.cattle.io/v1"
	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"github.com/rancher/wrangler/pkg/name"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
)

type handler struct {
	clusterCache      rocontrollers.ClusterCache
	projectCache      rocontrollers.ProjectCache
	projectController rocontrollers.ProjectController
}

func Register(ctx context.Context, clients *clients.Clients) {
	h := handler{
		clusterCache:      clients.Cluster.Cluster().Cache(),
		projectCache:      clients.Cluster.Project().Cache(),
		projectController: clients.Cluster.Project(),
	}

	rocontrollers.RegisterProjectGeneratingHandler(ctx,
		clients.Cluster.Project(),
		clients.Apply.
			WithCacheTypes(clients.Management.Project()),
		"",
		"project-create",
		h.onProject,
		nil)

	clients.Cluster.Cluster().OnChange(ctx, "project-cluster-trigger", h.onCluster)
}

func Projects(prj *v1.Project, clusterCache rocontrollers.ClusterCache) ([]*v3.Project, error) {
	if prj.Spec.ClusterSelector == nil {
		return nil, nil
	}

	sel, err := metav1.LabelSelectorAsSelector(prj.Spec.ClusterSelector)
	if err != nil {
		return nil, err
	}

	clusters, err := clusterCache.List(prj.Namespace, sel)
	if err != nil {
		return nil, err
	}

	var objs []*v3.Project
	for _, cluster := range clusters {
		objs = append(objs, &v3.Project{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name.SafeConcatName("p", cluster.Name, prj.Name),
				Namespace: cluster.Status.ClusterName,
			},
			Spec: v3.ProjectSpec{
				DisplayName: prj.Name,
				Description: prj.Annotations["field.cattle.io/description"],
				ClusterName: cluster.Status.ClusterName,
			},
		})
	}

	return objs, err
}

func (h *handler) onCluster(key string, cluster *v1.Cluster) (*v1.Cluster, error) {
	if cluster == nil {
		return cluster, nil
	}

	prjs, err := h.projectCache.List(cluster.Namespace, labels.Everything())
	if err != nil {
		return nil, err
	}

	var errs []error
	for _, prj := range prjs {
		if prj.Spec.ClusterSelector == nil {
			continue
		}
		sel, err := metav1.LabelSelectorAsSelector(prj.Spec.ClusterSelector)
		if err != nil {
			errs = append(errs, err)
		}
		if sel.Matches(labels.Set(cluster.Labels)) {
			h.projectController.Enqueue(prj.Namespace, prj.Name)
		}
	}

	return cluster, nil
}

func (h *handler) onProject(prj *v1.Project, status v1.ProjectStatus) ([]runtime.Object, v1.ProjectStatus, error) {
	prjs, err := Projects(prj, h.clusterCache)
	if err != nil {
		return nil, status, err
	}

	var result []runtime.Object
	for _, prj := range prjs {
		result = append(result, prj)
	}
	return result, status, nil
}
