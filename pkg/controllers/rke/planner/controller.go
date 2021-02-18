package planner

import (
	"context"
	"errors"

	rkev1 "github.com/rancher/rancher-operator/pkg/apis/rke.cattle.io/v1"
	"github.com/rancher/rancher-operator/pkg/clients"
	"github.com/rancher/rancher-operator/pkg/controllers/rke/machine"
	v1 "github.com/rancher/rancher-operator/pkg/generated/controllers/rke.cattle.io/v1"
	"github.com/rancher/rancher-operator/pkg/planner"
	"github.com/rancher/wrangler/pkg/relatedresource"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	capi "sigs.k8s.io/cluster-api/api/v1alpha4"
)

type handler struct {
	planner *planner.Planner
}

func Register(ctx context.Context, clients *clients.Clients) {
	h := handler{
		planner: planner.New(ctx, clients),
	}
	v1.RegisterRKEClusterStatusHandler(ctx,
		clients.RKE.RKECluster(), "", "planner", h.OnChange)
	relatedresource.Watch(ctx, "planner", func(namespace, name string, obj runtime.Object) ([]relatedresource.Key, error) {
		if secret, ok := obj.(*corev1.Secret); ok {
			clusterName := secret.Labels[machine.ClusterNameLabel]
			if clusterName != "" {
				return []relatedresource.Key{{
					Namespace: secret.Namespace,
					Name:      clusterName,
				}}, nil
			}
		} else if machine, ok := obj.(*capi.Machine); ok {
			return []relatedresource.Key{{
				Namespace: machine.Namespace,
				Name:      machine.Spec.ClusterName,
			}}, nil
		}
		return nil, nil
	}, clients.RKE.RKECluster(), clients.Core.Secret(), clients.CAPI.Machine())
}

func (h *handler) OnChange(cluster *rkev1.RKECluster, status rkev1.RKEClusterStatus) (rkev1.RKEClusterStatus, error) {
	status, err := h.planner.Process(cluster)
	if errors.Is(err, planner.ErrWaiting) {
		logrus.Info(err)
		return status, nil
	}
	return status, err
}
