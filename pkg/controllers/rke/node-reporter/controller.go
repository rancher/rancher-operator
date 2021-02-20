package nodereporter

import (
	"context"
	"fmt"

	"github.com/rancher/lasso/pkg/dynamic"
	"github.com/rancher/rancher-operator/pkg/clients"
	capicontrollers "github.com/rancher/rancher-operator/pkg/generated/controllers/cluster.x-k8s.io/v1alpha4"
	rkecontroller "github.com/rancher/rancher-operator/pkg/generated/controllers/rke.cattle.io/v1"
	"github.com/rancher/rancher-operator/pkg/planner"
	"github.com/rancher/rancher-operator/pkg/util"
	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	corev1 "k8s.io/api/core/v1"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	capi "sigs.k8s.io/cluster-api/api/v1alpha4"
)

const (
	machineUID = "machineUID"
)

type handler struct {
	machineCache     capicontrollers.MachineCache
	machines         capicontrollers.MachineClient
	capiClusterCache capicontrollers.ClusterCache
	rkeClusterCache  rkecontroller.RKEClusterCache
	dynamic          *dynamic.Controller
}

func Register(ctx context.Context, clients *clients.Clients) {
	clients.CAPI.Machine().Cache().AddIndexer(machineUID, func(obj *capi.Machine) ([]string, error) {
		return []string{string(obj.UID)}, nil
	})

	h := &handler{
		machineCache:     clients.CAPI.Machine().Cache(),
		machines:         clients.CAPI.Machine(),
		capiClusterCache: clients.CAPI.Cluster().Cache(),
		rkeClusterCache:  clients.RKE.RKECluster().Cache(),
		dynamic:          clients.Dynamic,
	}

	clients.Management.Node().OnChange(ctx, "node-reporter", h.OnChange)
}

func (h *handler) OnChange(key string, node *v3.Node) (*v3.Node, error) {
	if node == nil {
		return nil, nil
	}

	uid := node.Status.NodeLabels["rke.cattle.io/machine"]
	if uid == "" {
		return node, nil
	}

	machines, err := h.machineCache.GetByIndex(machineUID, uid)
	if apierror.IsNotFound(err) {
		return node, nil
	} else if err != nil {
		return node, err
	}

	for _, machine := range machines {
		if ok, err := h.sameCluster(node, machine); apierror.IsNotFound(err) {
			return node, nil
		} else if err != nil {
			return node, err
		} else if ok {
			return node, h.updateMachine(node, machine)
		}
	}

	return node, nil
}

func (h *handler) updateMachineJoinURL(node *v3.Node, machine *capi.Machine) error {
	address := ""
	for _, nodeAddress := range node.Status.InternalNodeStatus.Addresses {
		switch nodeAddress.Type {
		case corev1.NodeInternalIP:
			address = nodeAddress.Address
		case corev1.NodeExternalIP:
			if address == "" {
				address = nodeAddress.Address
			}
		}
	}

	url := fmt.Sprintf("https://%s:6443", address)
	if machine.Annotations[planner.JoinURLAnnotation] == url {
		return nil
	}

	machine = machine.DeepCopy()
	if machine.Annotations == nil {
		machine.Annotations = map[string]string{}
	}

	machine.Annotations[planner.JoinURLAnnotation] = url
	_, err := h.machines.Update(machine)
	return err
}

func (h *handler) updateMachine(node *v3.Node, machine *capi.Machine) error {
	if err := h.updateMachineJoinURL(node, machine); err != nil {
		return err
	}

	gvk := schema.FromAPIVersionAndKind(machine.Spec.InfrastructureRef.APIVersion, machine.Spec.InfrastructureRef.Kind)
	infra, err := h.dynamic.Get(gvk, machine.Namespace, machine.Spec.InfrastructureRef.Name)
	if apierror.IsNotFound(err) {
		return nil
	} else if err != nil {
		return err
	}

	data, err := util.ToMap(infra)
	if err != nil {
		return err
	}

	if data.String("spec", "providerID") != node.Spec.InternalNodeSpec.ProviderID {
		data, err := util.ToMap(infra.DeepCopyObject())
		if err != nil {
			return err
		}

		data.SetNested(node.Spec.InternalNodeSpec.ProviderID, "spec", "providerID")
		_, err = h.dynamic.Update(&unstructured.Unstructured{
			Object: data,
		})
		return err
	}

	return nil
}

func (h *handler) sameCluster(node *v3.Node, machine *capi.Machine) (bool, error) {
	capiCluster, err := h.capiClusterCache.Get(machine.Namespace, machine.Spec.ClusterName)
	if err != nil {
		return false, err
	}

	if capiCluster.Spec.InfrastructureRef == nil ||
		capiCluster.Spec.InfrastructureRef.APIVersion != "rke.cattle.io/v1" ||
		capiCluster.Spec.InfrastructureRef.Kind != "RKECluster" {
		return false, nil
	}

	rkeCluster, err := h.rkeClusterCache.Get(machine.Namespace, capiCluster.Spec.InfrastructureRef.Name)
	if err != nil {
		return false, err
	}

	return rkeCluster.Spec.ManagementClusterName == node.Namespace, nil
}
