package workspace

import (
	"context"

	fleet "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	"github.com/rancher/rancher-operator/pkg/clients"
	mgmtcontrollers "github.com/rancher/rancher-operator/pkg/generated/controllers/management.cattle.io/v3"
	mgmt "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"github.com/rancher/wrangler/pkg/generic"
	"github.com/rancher/wrangler/pkg/yaml"
	corev1 "k8s.io/api/core/v1"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	managed = "rancher.cattle.io/managed"
)

type handle struct {
	workspaceCache mgmtcontrollers.FleetWorkspaceCache
	workspaces     mgmtcontrollers.FleetWorkspaceClient
}

func Register(ctx context.Context, clients *clients.Clients) {
	h := &handle{
		workspaceCache: clients.Management.FleetWorkspace().Cache(),
		workspaces:     clients.Management.FleetWorkspace(),
	}

	mgmtcontrollers.RegisterFleetWorkspaceGeneratingHandler(ctx,
		clients.Management.FleetWorkspace(),
		clients.Apply.
			WithCacheTypes(clients.Core.Namespace()),
		"",
		"workspace",
		h.OnChange,
		&generic.GeneratingHandlerOptions{
			AllowClusterScoped: true,
		})

	clients.Fleet.ClusterRegistrationToken().OnChange(ctx, "workspace-backport",
		func(s string, token *fleet.ClusterRegistrationToken) (*fleet.ClusterRegistrationToken, error) {
			return token, h.onFleetObject(token)
		})
	clients.Fleet.Cluster().OnChange(ctx, "workspace-backport",
		func(s string, token *fleet.Cluster) (*fleet.Cluster, error) {
			return token, h.onFleetObject(token)
		})
	clients.Fleet.ClusterGroup().OnChange(ctx, "workspace-backport",
		func(s string, token *fleet.ClusterGroup) (*fleet.ClusterGroup, error) {
			return token, h.onFleetObject(token)
		})
	clients.Fleet.GitRepo().OnChange(ctx, "workspace-backport",
		func(s string, token *fleet.GitRepo) (*fleet.GitRepo, error) {
			return token, h.onFleetObject(token)
		})
}

func (h *handle) OnChange(workspace *mgmt.FleetWorkspace, status mgmt.FleetWorkspaceStatus) ([]runtime.Object, mgmt.FleetWorkspaceStatus, error) {
	if workspace.Annotations[managed] == "false" {
		return nil, status, nil
	}

	return []runtime.Object{
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   workspace.Name,
				Labels: yaml.CleanAnnotationsForExport(workspace.Labels),
			},
		},
	}, status, nil
}

func (h *handle) onFleetObject(obj runtime.Object) error {
	m, err := meta.Accessor(obj)
	if err != nil {
		// ignore error, this will happen when obj is nil
		return nil
	}

	_, err = h.workspaceCache.Get(m.GetNamespace())
	if apierror.IsNotFound(err) {
		_, err = h.workspaces.Create(&mgmt.FleetWorkspace{
			ObjectMeta: metav1.ObjectMeta{
				Name: m.GetNamespace(),
				Annotations: map[string]string{
					managed: "false",
				},
			},
			Status: mgmt.FleetWorkspaceStatus{},
		})
		if apierror.IsAlreadyExists(err) {
			err = nil
		}
	}

	return err
}
