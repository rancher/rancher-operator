package machine

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"github.com/rancher/lasso/pkg/dynamic"
	v1 "github.com/rancher/rancher-operator/pkg/apis/rancher.cattle.io/v1"
	rkev1 "github.com/rancher/rancher-operator/pkg/apis/rke.cattle.io/v1"
	"github.com/rancher/rancher-operator/pkg/clients"
	capicontrollers "github.com/rancher/rancher-operator/pkg/generated/controllers/cluster.x-k8s.io/v1alpha4"
	mgmtcontrollers "github.com/rancher/rancher-operator/pkg/generated/controllers/management.cattle.io/v3"
	ranchercontrollers "github.com/rancher/rancher-operator/pkg/generated/controllers/rancher.cattle.io/v1"
	rkecontroller "github.com/rancher/rancher-operator/pkg/generated/controllers/rke.cattle.io/v1"
	"github.com/rancher/rancher-operator/pkg/kubeconfig"
	"github.com/rancher/rancher-operator/pkg/planner"
	"github.com/rancher/rancher-operator/pkg/util"
	corecontrollers "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/name"
	"github.com/rancher/wrangler/pkg/relatedresource"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	capi "sigs.k8s.io/cluster-api/api/v1alpha4"
)

const (
	ClusterNameLabel = "rke.cattle.io/cluster-name"
	roleLabel        = "rke.cattle.io/service-account-role"
	planSecret       = "rke.cattle.io/plan-secret-name"
	roleBootstrap    = "bootstrap"
	rolePlan         = "plan"
)

var (
	bootstrapAPIVersion = fmt.Sprintf("%s/%s", rkev1.SchemeGroupVersion.Group, rkev1.SchemeGroupVersion.Version)
)

type handler struct {
	serviceAccountCache corecontrollers.ServiceAccountCache
	secretCache         corecontrollers.SecretCache
	capiClusterCache    capicontrollers.ClusterCache
	rancherClusterCache ranchercontrollers.ClusterCache
	machines            capicontrollers.MachineClient
	settingsCache       mgmtcontrollers.SettingCache
	rkeBootstrapCache   rkecontroller.RKEBootstrapCache
	rkeBootstrap        rkecontroller.RKEBootstrapClient
	kubeconfigManager   *kubeconfig.Manager
	dynamic             *dynamic.Controller
}

func Register(ctx context.Context, clients *clients.Clients) {
	h := &handler{
		serviceAccountCache: clients.Core.ServiceAccount().Cache(),
		secretCache:         clients.Core.Secret().Cache(),
		capiClusterCache:    clients.CAPI.Cluster().Cache(),
		rancherClusterCache: clients.Cluster.Cluster().Cache(),
		machines:            clients.CAPI.Machine(),
		settingsCache:       clients.Management.Setting().Cache(),
		rkeBootstrapCache:   clients.RKE.RKEBootstrap().Cache(),
		rkeBootstrap:        clients.RKE.RKEBootstrap(),
		kubeconfigManager:   kubeconfig.New(clients),
		dynamic:             clients.Dynamic,
	}
	capicontrollers.RegisterMachineGeneratingHandler(ctx,
		clients.CAPI.Machine(),
		clients.Apply.
			WithCacheTypes(
				clients.RBAC.Role(),
				clients.RBAC.RoleBinding(),
				clients.CAPI.Machine(),
				clients.Core.ServiceAccount(),
				clients.Core.Secret()).
			WithSetOwnerReference(false, false),
		"",
		"rke-machine",
		h.OnChange,
		nil)

	relatedresource.Watch(ctx, "rke-machine-trigger", func(namespace, name string, obj runtime.Object) ([]relatedresource.Key, error) {
		if sa, ok := obj.(*corev1.ServiceAccount); ok {
			if name, ok := sa.Labels[planner.MachineNameLabel]; ok {
				return []relatedresource.Key{
					{
						Namespace: sa.Namespace,
						Name:      name,
					},
				}, nil
			}
		}
		return nil, nil
	}, clients.CAPI.Machine(), clients.Core.ServiceAccount())

	clients.CAPI.Machine().OnChange(ctx, "machine-provider-sync", h.associateMachineWithNode)
}

func IsRKECluster(spec *capi.ClusterSpec) bool {
	if spec.InfrastructureRef == nil {
		return false
	}
	gvk := schema.FromAPIVersionAndKind(spec.InfrastructureRef.APIVersion, spec.InfrastructureRef.Kind)
	return gvk.Group == rkev1.SchemeGroupVersion.Group &&
		spec.InfrastructureRef.Kind == "RKECluster"
}

func (h *handler) getBootstrapSecret(namespace, name string) (*corev1.Secret, error) {
	sa, err := h.serviceAccountCache.Get(namespace, name)
	if apierror.IsNotFound(err) {
		return nil, nil
	}

	if err != nil {
		return nil, err

	}
	for _, secretRef := range sa.Secrets {
		secret, err := h.secretCache.Get(sa.Namespace, secretRef.Name)
		if err != nil {
			return nil, err
		}

		hash := sha256.Sum256(secret.Data["token"])
		data, err := Bootstrap(h.settingsCache, base64.URLEncoding.EncodeToString(hash[:]))
		if err != nil {
			return nil, err
		}

		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Data: map[string][]byte{
				"value": data,
			},
			Type: "rke.cattle.io/bootstrap",
		}, nil
	}

	return nil, nil
}

func (h *handler) assignPlanSecret(obj *capi.Machine) ([]runtime.Object, error) {
	secretName := planner.PlanSecretFromMachine(obj)

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: obj.Namespace,
			Labels: map[string]string{
				ClusterNameLabel:         obj.Spec.ClusterName,
				planner.MachineNameLabel: obj.Name,
				roleLabel:                rolePlan,
				planSecret:               secretName,
			},
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: obj.Namespace,
			Labels: map[string]string{
				planner.MachineNameLabel: obj.Name,
			},
		},
		Type: planner.SecretTypeMachinePlan,
	}
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: obj.Namespace,
		},
		Rules: []rbacv1.PolicyRule{
			{
				Verbs:         []string{"watch", "get", "update", "list"},
				APIGroups:     []string{""},
				Resources:     []string{"secrets"},
				ResourceNames: []string{secretName},
			},
		},
	}
	rolebinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: obj.Namespace,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      sa.Name,
				Namespace: sa.Namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "Role",
			Name:     secretName,
		},
	}

	return []runtime.Object{sa, secret, role, rolebinding}, nil
}

func (h *handler) assignBootStrapSecret(obj *capi.Machine) (*corev1.Secret, []runtime.Object, error) {
	if obj.Spec.Bootstrap.ConfigRef == nil ||
		obj.Spec.Bootstrap.ConfigRef.APIVersion != bootstrapAPIVersion ||
		obj.Spec.Bootstrap.ConfigRef.Kind != "RKEBootstrap" {
		return nil, nil, nil
	}

	if capi.MachinePhase(obj.Status.Phase) != capi.MachinePhasePending &&
		capi.MachinePhase(obj.Status.Phase) != capi.MachinePhaseDeleting &&
		capi.MachinePhase(obj.Status.Phase) != capi.MachinePhaseFailed &&
		capi.MachinePhase(obj.Status.Phase) != capi.MachinePhaseProvisioning {
		return nil, nil, nil
	}

	secretName := name.SafeConcatName(obj.Name, "machine", "bootstrap")

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: obj.Namespace,
			Labels: map[string]string{
				planner.MachineNameLabel: obj.Name,
				roleLabel:                roleBootstrap,
			},
		},
	}

	bootstrapSecret, err := h.getBootstrapSecret(sa.Namespace, sa.Name)
	if err != nil {
		return nil, nil, err
	}

	if bootstrapSecret != nil {
		rkeBootstrap, err := h.rkeBootstrapCache.Get(obj.Namespace, obj.Spec.Bootstrap.ConfigRef.Name)
		if err != nil {
			return nil, nil, err
		}

		if rkeBootstrap.Status.DataSecretName == nil || *rkeBootstrap.Status.DataSecretName != bootstrapSecret.Name {
			rkeBootstrap = rkeBootstrap.DeepCopy()
			rkeBootstrap.Status.DataSecretName = &bootstrapSecret.Name
			rkeBootstrap.Status.Ready = true
			if _, err := h.rkeBootstrap.UpdateStatus(rkeBootstrap); err != nil {
				return nil, nil, err
			}
		}
	}

	return bootstrapSecret, []runtime.Object{sa}, nil
}

func (h *handler) OnChange(obj *capi.Machine, status capi.MachineStatus) ([]runtime.Object, capi.MachineStatus, error) {
	var (
		result []runtime.Object
	)

	cluster, err := h.capiClusterCache.Get(obj.Namespace, obj.Spec.ClusterName)
	if err != nil {
		return nil, status, err
	}

	if !IsRKECluster(&cluster.Spec) {
		return nil, status, nil
	}

	objs, err := h.assignPlanSecret(obj)
	if err != nil {
		return nil, status, err
	}

	result = append(result, objs...)

	bootstrapSecret, objs, err := h.assignBootStrapSecret(obj)
	if err != nil {
		return nil, status, err
	}

	if bootstrapSecret != nil {
		result = append(result, bootstrapSecret)
	}

	result = append(result, objs...)
	return result, status, nil
}

func (h *handler) associateMachineWithNode(_ string, machine *capi.Machine) (*capi.Machine, error) {
	if machine == nil || machine.DeletionTimestamp != nil {
		return machine, nil
	}

	if machine.Spec.ProviderID != nil && *machine.Spec.ProviderID != "" {
		// If the machine already has its provider ID set, then we do not need to continue
		return machine, nil
	}

	rancherCluster, err := h.getAssociatedClusters(machine)
	if err != nil {
		return machine, err
	}

	secret, err := h.kubeconfigManager.GetKubeConfig(rancherCluster, rancherCluster.Status)
	if err != nil {
		return machine, err
	}

	config, err := clientcmd.RESTConfigFromKubeConfig(secret.Data["value"])
	if err != nil {
		return machine, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return machine, err
	}

	nodes, err := clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{LabelSelector: "rke.cattle.io/machine"})
	if err != nil {
		return machine, err
	}

	for _, node := range nodes.Items {
		if node.Labels["rke.cattle.io/machine"] != string(machine.GetUID()) {
			continue
		}
		return machine, h.updateMachine(&node, machine, rancherCluster)
	}

	return machine, fmt.Errorf("no node found for machine %v", machine.Name)
}

func (h *handler) updateMachineJoinURL(node *corev1.Node, machine *capi.Machine, rancherCluster *v1.Cluster) error {
	address := ""
	for _, nodeAddress := range node.Status.Addresses {
		switch nodeAddress.Type {
		case corev1.NodeInternalIP:
			address = nodeAddress.Address
		case corev1.NodeExternalIP:
			if address == "" {
				address = nodeAddress.Address
			}
		}
	}

	url := fmt.Sprintf("https://%s:%d", address, getJoinURLPort(rancherCluster))
	if machine.Annotations[planner.JoinURLAnnotation] == url {
		return nil
	}

	machine = machine.DeepCopy()
	if machine.Annotations == nil {
		machine.Annotations = map[string]string{}
	}

	machine.Annotations[planner.JoinURLAnnotation] = url
	machine.Annotations["rke.cattle.io/node-ip-address"] = address
	_, err := h.machines.Update(machine)
	return err
}

func (h *handler) updateMachine(node *corev1.Node, machine *capi.Machine, rancherCluster *v1.Cluster) error {
	if err := h.updateMachineJoinURL(node, machine, rancherCluster); err != nil {
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

	if data.String("spec", "providerID") != node.Spec.ProviderID {
		data, err := util.ToMap(infra.DeepCopyObject())
		if err != nil {
			return err
		}

		data.SetNested(node.Spec.ProviderID, "spec", "providerID")
		_, err = h.dynamic.Update(&unstructured.Unstructured{
			Object: data,
		})
		return err
	}

	return nil
}

func (h *handler) getAssociatedClusters(machine *capi.Machine) (*v1.Cluster, error) {
	rancherCluster, err := h.rancherClusterCache.Get(machine.Namespace, machine.Spec.ClusterName)
	if err != nil {
		return nil, err
	}

	return rancherCluster, err
}

func getJoinURLPort(cluster *v1.Cluster) int {
	if planner.GetRuntime(cluster.Spec.KubernetesVersion) == planner.RuntimeRKE2 {
		return 9345
	}
	return 6443
}
