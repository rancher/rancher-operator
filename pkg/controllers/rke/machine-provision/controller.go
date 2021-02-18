package machineprovision

import (
	"context"
	"strings"

	"github.com/rancher/lasso/pkg/dynamic"
	rkev1 "github.com/rancher/rancher-operator/pkg/apis/rke.cattle.io/v1"
	"github.com/rancher/rancher-operator/pkg/clients"
	capicontrollers "github.com/rancher/rancher-operator/pkg/generated/controllers/cluster.x-k8s.io/v1alpha4"
	mgmtcontrollers "github.com/rancher/rancher-operator/pkg/generated/controllers/management.cattle.io/v3"
	rkecontroller "github.com/rancher/rancher-operator/pkg/generated/controllers/rke.cattle.io/v1"
	"github.com/rancher/rancher-operator/pkg/util"
	"github.com/rancher/wrangler/pkg/apply"
	"github.com/rancher/wrangler/pkg/condition"
	"github.com/rancher/wrangler/pkg/data"
	"github.com/rancher/wrangler/pkg/data/convert"
	batchcontrollers "github.com/rancher/wrangler/pkg/generated/controllers/batch/v1"
	corecontrollers "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/generic"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cluster-api/errors"
)

type handler struct {
	ctx             context.Context
	apply           apply.Apply
	jobs            batchcontrollers.JobCache
	pods            corecontrollers.PodCache
	secrets         corecontrollers.SecretCache
	machines        capicontrollers.MachineCache
	clusters        capicontrollers.ClusterCache
	rkeClusters     rkecontroller.RKEClusterCache
	nodeDriverCache mgmtcontrollers.NodeDriverCache
	dynamic         *dynamic.Controller
}

func Register(ctx context.Context, clients *clients.Clients) {
	h := &handler{
		ctx: ctx,
		apply: clients.Apply.
			WithSetOwnerReference(false, false).
			WithCacheTypes(clients.Core.Secret(),
				clients.Core.ServiceAccount(),
				clients.RBAC.RoleBinding(),
				clients.RBAC.Role(),
				clients.Batch.Job()),
		pods:            clients.Core.Pod().Cache(),
		jobs:            clients.Batch.Job().Cache(),
		secrets:         clients.Core.Secret().Cache(),
		machines:        clients.CAPI.Machine().Cache(),
		clusters:        clients.CAPI.Cluster().Cache(),
		rkeClusters:     clients.RKE.RKECluster().Cache(),
		nodeDriverCache: clients.Management.NodeDriver().Cache(),
		dynamic:         clients.Dynamic,
	}

	removeHandler := generic.NewRemoveHandler("machine-provision-remove", clients.Dynamic.Update, h.OnRemove)

	clients.Dynamic.OnChange(ctx, "machine-provision-remove", validGVK, dynamic.FromKeyHandler(removeHandler))
	clients.Dynamic.OnChange(ctx, "machine-provision", validGVK, h.OnChange)
	clients.Batch.Job().OnChange(ctx, "machine-provision-pod", h.OnJobChange)
}

func validGVK(gvk schema.GroupVersionKind) bool {
	return gvk.Group == "rke-node.cattle.io" &&
		gvk.Version == "v1" &&
		strings.HasSuffix(gvk.Kind, "Machine") &&
		gvk.Kind != "UnmanagedMachine"
}

func (h *handler) OnJobChange(key string, job *batchv1.Job) (*batchv1.Job, error) {
	if job == nil {
		return nil, nil
	}

	name := job.Spec.Template.Labels[InfraMachineName]
	group := job.Spec.Template.Labels[InfraMachineGroup]
	version := job.Spec.Template.Labels[InfraMachineVersion]
	kind := job.Spec.Template.Labels[InfraMachineKind]

	if name == "" || kind == "" {
		return job, nil
	}

	infraMachine, err := h.dynamic.Get(schema.GroupVersionKind{
		Group:   group,
		Version: version,
		Kind:    kind,
	}, job.Namespace, name)
	if apierror.IsNotFound(err) {
		// ignore err
		return job, nil
	} else if err != nil {
		return job, err
	}

	meta, err := meta.Accessor(infraMachine)
	if err != nil {
		return nil, err
	}

	data, err := util.ToMap(infraMachine)
	if err != nil {
		return job, err
	}

	newStatus, err := h.getMachineStatus(job)
	if err != nil {
		return job, err
	}

	if _, err := h.patchStatus(infraMachine, data, newStatus); err != nil {
		return job, err
	}

	// Re-evaluate the infra-machine after this
	if err := h.dynamic.Enqueue(infraMachine.GetObjectKind().GroupVersionKind(),
		meta.GetNamespace(), meta.GetName()); err != nil {
		return nil, err
	}

	return job, nil
}

func (h *handler) getMachineStatus(job *batchv1.Job) (rkev1.RKEMachineStatus, error) {
	if job.Status.CompletionTime != nil {
		return rkev1.RKEMachineStatus{
			JobComplete: true,
		}, nil
	}

	if condition.Cond("Failed").IsTrue(job) {
		sel, err := metav1.LabelSelectorAsSelector(job.Spec.Selector)
		if err != nil {
			return rkev1.RKEMachineStatus{}, err
		}

		pods, err := h.pods.List(job.Namespace, sel)
		if err != nil {
			return rkev1.RKEMachineStatus{}, err
		}

		var lastPod *corev1.Pod
		for _, pod := range pods {
			if lastPod == nil {
				lastPod = pod
				continue
			} else if pod.CreationTimestamp.After(lastPod.CreationTimestamp.Time) {
				lastPod = pod
			}
		}

		if lastPod != nil {
			return getMachineStatusFromPod(lastPod), nil
		}
	}

	return rkev1.RKEMachineStatus{}, nil
}

func getMachineStatusFromPod(pod *corev1.Pod) rkev1.RKEMachineStatus {
	if pod.Status.Phase == corev1.PodSucceeded {
		return rkev1.RKEMachineStatus{
			JobComplete: true,
		}
	}

	for _, containerStatus := range pod.Status.ContainerStatuses {
		if containerStatus.State.Terminated != nil && containerStatus.State.Terminated.ExitCode != 0 {
			return rkev1.RKEMachineStatus{
				FailureReason:  string(errors.CreateMachineError),
				FailureMessage: strings.TrimSpace(pod.Status.ContainerStatuses[0].State.Terminated.Message),
			}
		}
	}

	return rkev1.RKEMachineStatus{}
}

func (h *handler) OnRemove(_ string, obj runtime.Object) (runtime.Object, error) {
	obj, err := h.run(obj, false)
	if err != nil {
		return nil, err
	}

	meta, err := meta.Accessor(obj)
	if err != nil {
		return nil, err
	}

	job, err := h.jobs.Get(meta.GetNamespace(), getJobName(meta.GetName()))
	if err != nil {
		return nil, err
	}

	if condition.Cond("Failed").IsTrue(job) || job.Status.CompletionTime != nil {
		return obj, nil
	}

	// ErrSkip will not remove finalizer but treat this as currently reconciled
	return nil, generic.ErrSkip
}

func (h *handler) OnChange(obj runtime.Object) (runtime.Object, error) {
	newObj, err := h.run(obj, true)
	if newObj == nil {
		newObj = obj
	}
	return util.SetCondition(h.dynamic, newObj, "CreateJob", err)
}

func (h *handler) run(obj runtime.Object, create bool) (runtime.Object, error) {
	typeMeta, err := meta.TypeAccessor(obj)
	if err != nil {
		return nil, err
	}

	meta, err := meta.Accessor(obj)
	if err != nil {
		return nil, err
	}

	// don't process create if deleting
	if create && meta.GetDeletionTimestamp() != nil {
		return obj, nil
	}

	data, err := util.ToMap(obj)
	if err != nil {
		return nil, err
	}

	args, err := h.getArgsEnvAndStatus(typeMeta, meta, data, create)
	if err != nil {
		return obj, err
	}

	objs, err := h.objects(data.Bool("status", "ready") && create, typeMeta, meta, args)
	if err != nil {
		return nil, err
	}

	if err := h.apply.WithOwner(obj).ApplyObjects(objs...); err != nil {
		return nil, err
	}

	if create {
		return h.patchStatus(obj, data, args.RKEMachineStatus)
	}

	return obj, h.apply.WithOwner(obj).ApplyObjects(objs...)
}

func (h *handler) patchStatus(obj runtime.Object, data data.Object, state rkev1.RKEMachineStatus) (runtime.Object, error) {
	statusData, err := convert.EncodeToMap(state)
	if err != nil {
		return nil, err
	}

	changed := false
	for k, v := range statusData {
		if data.String("status", k) != convert.ToString(v) {
			changed = true
			break
		}
	}

	if !changed {
		return obj, nil
	}

	data, err = util.ToMap(obj.DeepCopyObject())
	if err != nil {
		return nil, err
	}

	status := data.Map("status")
	if status == nil {
		status = map[string]interface{}{}
		data.Set("status", status)
	}
	for k, v := range statusData {
		status[k] = v
	}

	return h.dynamic.UpdateStatus(&unstructured.Unstructured{
		Object: data,
	})
}
