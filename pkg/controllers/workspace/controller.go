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

	clients.Management.Setting().OnChange(ctx, "default-workspace", h.OnSetting)

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
		func(s string, obj *fleet.ClusterRegistrationToken) (*fleet.ClusterRegistrationToken, error) {
			if obj == nil {
				return nil, nil
			}
			return obj, h.onFleetObject(obj)
		})
	clients.Fleet.Cluster().OnChange(ctx, "workspace-backport",
		func(s string, obj *fleet.Cluster) (*fleet.Cluster, error) {
			if obj == nil {
				return nil, nil
			}
			return obj, h.onFleetObject(obj)
		})
	clients.Fleet.ClusterGroup().OnChange(ctx, "workspace-backport",
		func(s string, obj *fleet.ClusterGroup) (*fleet.ClusterGroup, error) {
			if obj == nil {
				return nil, nil
			}
			return obj, h.onFleetObject(obj)
		})
	clients.Fleet.GitRepo().OnChange(ctx, "workspace-backport",
		func(s string, obj *fleet.GitRepo) (*fleet.GitRepo, error) {
			if obj == nil {
				return nil, nil
			}
			return obj, h.onFleetObject(obj)
		})
}

func (h *handle) OnSetting(key string, setting *mgmt.Setting) (*mgmt.Setting, error) {
	if setting == nil || setting.Name != "fleet-default-workspace-name" {
		return setting, nil
	}

	value := setting.Value
	if value == "" {
		value = setting.Default
	}

	if value == "" {
		return setting, nil
	}

	_, err := h.workspaceCache.Get(value)
	if apierror.IsNotFound(err) {
		_, err = h.workspaces.Create(&mgmt.FleetWorkspace{
			ObjectMeta: metav1.ObjectMeta{
				Name: value,
			},
		})
	}

	return setting, err
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
		return err
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
