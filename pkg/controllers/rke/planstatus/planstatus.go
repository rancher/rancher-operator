package planstatus

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/rancher/rancher-operator/pkg/clients"
	capicontrollers "github.com/rancher/rancher-operator/pkg/generated/controllers/cluster.x-k8s.io/v1alpha4"
	"github.com/rancher/rancher-operator/pkg/planner"
	"github.com/rancher/wrangler/pkg/condition"
	corecontrollers "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	corev1 "k8s.io/api/core/v1"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	capi "sigs.k8s.io/cluster-api/api/v1alpha4"
)

const (
	Provisioned = condition.Cond("Provisioned")
)

type handler struct {
	secrets      corecontrollers.SecretClient
	machines     capicontrollers.MachineClient
	machineCache capicontrollers.MachineCache
}

func Register(ctx context.Context, clients *clients.Clients) {
	h := handler{
		secrets:      clients.Core.Secret(),
		machines:     clients.CAPI.Machine(),
		machineCache: clients.CAPI.Machine().Cache(),
	}
	clients.Core.Secret().OnChange(ctx, "plan-status", h.OnChange)
}

func (h *handler) updateMachineProvisionStatus(secret *corev1.Secret) error {
	machineName := secret.Labels[planner.MachineNameLabel]
	if machineName == "" {
		return nil
	}

	machine, err := h.machineCache.Get(secret.Namespace, machineName)
	if apierror.IsNotFound(err) {
		return nil
	} else if err != nil {
		return err
	}

	// make sure there's no funny business going on here
	if planner.PlanSecretFromMachine(machine) != secret.Name {
		return nil
	}

	plan, err := planner.SecretToNode(secret)
	if err != nil {
		return err
	}

	status, reason, message := planner.GetPlanStatusReasonMessage(plan)
	if corev1.ConditionStatus(Provisioned.GetStatus(machine)) != status ||
		Provisioned.GetReason(machine) != string(reason) ||
		Provisioned.GetMessage(machine) != message {
		machine := machine.DeepCopy()
		newCond := capi.Condition{
			Type:               capi.ConditionType(Provisioned),
			Status:             status,
			LastTransitionTime: metav1.Now(),
			Reason:             string(reason),
			Message:            message,
		}
		if status == corev1.ConditionFalse {
			newCond.Severity = capi.ConditionSeverityError
		} else {
			newCond.Severity = capi.ConditionSeverityInfo
		}

		set := false
		for i, cond := range machine.Status.Conditions {
			if string(cond.Type) == string(Provisioned) {
				set = true
				machine.Status.Conditions[i] = newCond
				break
			}
		}

		if !set {
			machine.Status.Conditions = append(machine.Status.Conditions, newCond)
		}

		_, err := h.machines.UpdateStatus(machine)
		if err != nil {
			return err
		}
	}

	return nil
}

func (h *handler) OnChange(key string, secret *corev1.Secret) (*corev1.Secret, error) {
	if secret == nil || secret.Type != planner.SecretTypeMachinePlan || len(secret.Data) == 0 {
		return secret, nil
	}

	if err := h.updateMachineProvisionStatus(secret); err != nil {
		return secret, err
	}

	if len(secret.Data) == 0 {
		return secret, nil
	}

	appliedChecksum := string(secret.Data["applied-checksum"])
	plan := secret.Data["plan"]
	appliedPlan := secret.Data["appliedPlan"]

	if appliedChecksum == hash(plan) {
		if !bytes.Equal(plan, appliedPlan) {
			secret = secret.DeepCopy()
			secret.Data["appliedPlan"] = plan
			return h.secrets.Update(secret)
		}
	}

	return secret, nil
}

func hash(plan []byte) string {
	result := sha256.Sum256(plan)
	return hex.EncodeToString(result[:])
}
