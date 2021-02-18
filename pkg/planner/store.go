package planner

import (
	"bytes"
	"encoding/json"

	rkev1 "github.com/rancher/rancher-operator/pkg/apis/rke.cattle.io/v1"
	"github.com/rancher/rancher-operator/pkg/apis/rke.cattle.io/v1/plan"
	capicontrollers "github.com/rancher/rancher-operator/pkg/generated/controllers/cluster.x-k8s.io/v1alpha4"
	corecontrollers "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	corev1 "k8s.io/api/core/v1"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	capi "sigs.k8s.io/cluster-api/api/v1alpha4"
)

type planStore struct {
	secrets      corecontrollers.SecretClient
	secretsCache corecontrollers.SecretCache
	machineCache capicontrollers.MachineCache
}

func (p *planStore) Load(cluster *rkev1.RKECluster) (*plan.Plan, error) {
	result := &plan.Plan{
		Nodes:    map[string]*plan.Node{},
		Machines: map[string]*capi.Machine{},
		Cluster:  cluster,
	}

	machines, err := p.machineCache.List(cluster.Namespace, labels.SelectorFromSet(map[string]string{
		capiMachineLabel: cluster.Name,
	}))
	if err != nil {
		return nil, err
	}

	secrets, err := p.getSecrets(machines)
	if err != nil {
		return nil, err
	}

	for _, machine := range machines {
		result.Machines[machine.Name] = machine
	}

	for machineName, secret := range secrets {
		node, err := p.secretToNode(secret)
		if err != nil {
			return nil, err
		}
		if node == nil {
			continue
		}
		result.Nodes[machineName] = node
	}

	return result, nil
}

func (p *planStore) secretToNode(secret *corev1.Secret) (*plan.Node, error) {
	result := &plan.Node{}
	planData := secret.Data["plan"]
	appliedPlanData := secret.Data["appliedPlan"]

	if len(planData) > 0 {
		if err := json.Unmarshal(planData, &result.Plan); err != nil {
			return nil, err
		}
	} else {
		return nil, nil
	}

	if len(appliedPlanData) > 0 {
		newPlan := &plan.NodePlan{}
		if err := json.Unmarshal(appliedPlanData, newPlan); err != nil {
			return nil, err
		}
		result.AppliedPlan = newPlan
	}

	result.InSync = bytes.Equal(planData, appliedPlanData)
	return result, nil
}

func (p *planStore) getSecrets(machines []*capi.Machine) (map[string]*corev1.Secret, error) {
	result := map[string]*corev1.Secret{}
	for _, machine := range machines {
		secret, err := p.secretsCache.Get(machine.Namespace, PlanSecretFromMachine(machine))
		if apierror.IsNotFound(err) {
			continue
		} else if err != nil {
			return nil, err
		}

		result[machine.Name] = secret
	}

	return result, nil
}

func (p *planStore) UpdatePlan(machine *capi.Machine, plan plan.NodePlan) error {
	data, err := json.Marshal(plan)
	if err != nil {
		return err
	}

	secret, err := p.secrets.Get(machine.Namespace, PlanSecretFromMachine(machine), metav1.GetOptions{})
	if err != nil {
		return err
	}

	if secret.Data == nil {
		secret.Data = map[string][]byte{}
	}

	secret.Data["plan"] = data
	_, err = p.secrets.Update(secret)
	return err
}
