package planner

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	rkev1 "github.com/rancher/rancher-operator/pkg/apis/rke.cattle.io/v1"
	"github.com/rancher/rancher-operator/pkg/apis/rke.cattle.io/v1/plan"
	"github.com/rancher/rancher-operator/pkg/clients"
	capicontrollers "github.com/rancher/rancher-operator/pkg/generated/controllers/cluster.x-k8s.io/v1alpha4"
	mgmtcontrollers "github.com/rancher/rancher-operator/pkg/generated/controllers/management.cattle.io/v3"
	"github.com/rancher/rancher-operator/pkg/kubeconfig"
	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	corecontrollers "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/name"
	"github.com/rancher/wrangler/pkg/randomtoken"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	capi "sigs.k8s.io/cluster-api/api/v1alpha4"
)

const (
	clusterRegToken   = "clusterRegToken"
	JoinURLAnnotation = "rke.cattle.io/join-url"

	InitNodeLabel         = "rke.cattle.io/init-node"
	EtcdRoleLabel         = "rke.cattle.io/etcd-role"
	WorkerRoleLabel       = "rke.cattle.io/worker-role"
	ControlPlaneRoleLabel = "rke.cattle.io/control-plane-role"
	MachineUIDLabel       = "rke.cattle.io/machine"

	MachineNameLabel      = "rke.cattle.io/machine-name"
	MachineNamespaceLabel = "rke.cattle.io/machine-namespace"

	LabelsAnnotation = "rke.cattle.io/labels"
	TaintsAnnotation = "rke.cattle.io/taints"
)

var (
	capiMachineLabel = "cluster.x-k8s.io/cluster-name"
	ErrWaiting       = errors.New("waiting")
)

type roleFilter func(machine *capi.Machine) bool

type Planner struct {
	ctx                           context.Context
	store                         *planStore
	secretClient                  corecontrollers.SecretClient
	secretCache                   corecontrollers.SecretCache
	machines                      capicontrollers.MachineClient
	clusterRegistrationTokenCache mgmtcontrollers.ClusterRegistrationTokenCache
	settings                      mgmtcontrollers.SettingCache
	kubeconfig                    *kubeconfig.Manager
}

func New(ctx context.Context, clients *clients.Clients) *Planner {
	clients.Management.ClusterRegistrationToken().Cache().AddIndexer(clusterRegToken, func(obj *v3.ClusterRegistrationToken) ([]string, error) {
		return []string{obj.Spec.ClusterName}, nil
	})
	return &Planner{
		ctx: ctx,
		store: &planStore{
			secrets:      clients.Core.Secret(),
			secretsCache: clients.Core.Secret().Cache(),
			machineCache: clients.CAPI.Machine().Cache(),
		},
		machines:                      clients.CAPI.Machine(),
		secretClient:                  clients.Core.Secret(),
		secretCache:                   clients.Core.Secret().Cache(),
		clusterRegistrationTokenCache: clients.Management.ClusterRegistrationToken().Cache(),
		settings:                      clients.Management.Setting().Cache(),
		kubeconfig:                    kubeconfig.New(clients),
	}
}

func PlanSecretFromMachine(obj *capi.Machine) string {
	return name.SafeConcatName(obj.Name, "machine", "plan")
}

func (p *Planner) Process(cluster *rkev1.RKECluster) (rkev1.RKEClusterStatus, error) {
	plan, err := p.store.Load(cluster)
	if err != nil {
		return cluster.Status, err
	}

	cluster, secret, err := p.generateSecrets(cluster)
	if err != nil {
		return cluster.Status, err
	}

	if _, err := p.electInitNode(plan); err != nil {
		return cluster.Status, err
	}

	ok, err := p.reconcile(cluster, secret, plan, isInitNode, none, cluster.Spec.UpgradeStrategy.ServerConcurrency, "")
	if err != nil || !ok {
		return cluster.Status, err
	}

	joinServer, err := p.electInitNode(plan)
	if err != nil || joinServer == "" {
		return cluster.Status, err
	}

	ok, err = p.reconcile(cluster, secret, plan, isEtcd, isInitNode, cluster.Spec.UpgradeStrategy.ServerConcurrency, joinServer)
	if err != nil || !ok {
		return cluster.Status, err
	}

	ok, err = p.reconcile(cluster, secret, plan, isControlPlane, isInitNode, cluster.Spec.UpgradeStrategy.ServerConcurrency, joinServer)
	if err != nil || !ok {
		return cluster.Status, err
	}

	ok, err = p.reconcile(cluster, secret, plan, isOnlyWorker, isInitNode, cluster.Spec.UpgradeStrategy.WorkerConcurrency, joinServer)
	if err != nil || !ok {
		return cluster.Status, err
	}

	return cluster.Status, err
}

func (p *Planner) CurrentPlan(cluster *rkev1.RKECluster) (*plan.Plan, error) {
	return p.store.Load(cluster)
}

func (p *Planner) clearInitNodeMark(machine *capi.Machine) error {
	if _, ok := machine.Labels[InitNodeLabel]; !ok {
		return nil
	}
	machine = machine.DeepCopy()
	delete(machine.Labels, InitNodeLabel)
	_, err := p.machines.Update(machine)
	return err
}

func (p *Planner) setInitNodeMark(machine *capi.Machine) (*capi.Machine, error) {
	if machine.Labels[InitNodeLabel] == "true" {
		return machine, nil
	}
	machine = machine.DeepCopy()
	if machine.Labels == nil {
		machine.Labels = map[string]string{}
	}
	machine.Labels[InitNodeLabel] = "true"
	return p.machines.Update(machine)
}

func (p *Planner) electInitNode(plan *plan.Plan) (string, error) {
	entries, _ := collect(plan, isEtcd, none)
	joinURL := ""
	for _, entry := range entries {
		if !isInitNode(entry.Machine) {
			continue
		}

		// Clear old or misconfigured init nodes
		if entry.Machine.DeletionTimestamp != nil || joinURL != "" {
			if err := p.clearInitNodeMark(entry.Machine); err != nil {
				return "", err
			}
			continue
		}

		joinURL = entry.Machine.Annotations[JoinURLAnnotation]
	}

	if joinURL != "" {
		return joinURL, nil
	}

	if len(entries) == 0 {
		return "", nil
	}
	machine, err := p.setInitNodeMark(entries[0].Machine)
	if err != nil {
		return "", err
	}
	return machine.Annotations[JoinURLAnnotation], nil
}

func (p *Planner) reconcile(cluster *rkev1.RKECluster, secret plan.Secret, plan *plan.Plan, include, exclude roleFilter, concurrency int, joinServer string) (bool, error) {
	entries, unavailable := collect(plan, include, exclude)

	allInSync := true
	for _, entry := range entries {
		plan, err := p.desiredPlan(cluster, secret, entry, isInitNode(entry.Machine), joinServer)
		if err != nil {
			return false, err
		}

		if entry.Plan == nil {
			allInSync = false
			if err := p.store.UpdatePlan(entry.Machine, plan); err != nil {
				return false, err
			}
		} else if !equality.Semantic.DeepEqual(entry.Plan.Plan, plan) {
			allInSync = false
			if !entry.Plan.InSync || concurrency == 0 || unavailable < concurrency {
				if entry.Plan.InSync {
					unavailable++
				}
				if err := p.store.UpdatePlan(entry.Machine, plan); err != nil {
					return false, err
				}
			}
		} else if !entry.Plan.InSync {
			allInSync = false
		}
	}

	return allInSync && len(entries) > 0, nil
}

func (p *Planner) desiredPlan(cluster *rkev1.RKECluster, secret plan.Secret, entry planEntry, initNode bool, joinServer string) (result plan.NodePlan, _ error) {
	agent := false
	config := map[string]interface{}{}
	for _, opts := range cluster.Spec.Config {
		sel, err := metav1.LabelSelectorAsSelector(opts.MachineLabelSelector)
		if err != nil {
			return result, err
		}
		if sel.Matches(labels.Set(cluster.Labels)) {
			config = opts.Config.DeepCopy().Data
			break
		}
	}

	if initNode {
		config["cluster-init"] = true
	} else {
		config["server"] = joinServer
	}

	if isOnlyEtcd(entry.Machine) {
		config["role"] = "etcd"
	} else if isOnlyWorker(entry.Machine) {
		agent = true
	}

	if initNode {
		data, err := p.loadClusterAgent(cluster)
		if err != nil {
			return result, err
		}
		result.Files = append(result.Files, plan.File{
			Content: base64.StdEncoding.EncodeToString(data),
			Path:    fmt.Sprintf("/var/lib/rancher/%s/server/manifests/cluster-agent.yaml", p.getRuntime(cluster)),
		})
	}

	instruction := plan.Instruction{
		Image:   p.kubernetesVersionToImage(cluster.Spec.KubernetesVersion),
		Command: "sh",
		Args:    []string{"-c", "install.sh"},
	}

	if agent {
		instruction.Args = []string{"agent"}
		config["token"] = secret.AgentToken
	} else {
		config["token"] = secret.ServerToken
		config["agent-token"] = secret.AgentToken
	}

	var labels []string
	if data := entry.Machine.Annotations[LabelsAnnotation]; data != "" {
		labelMap := map[string]string{}
		if err := json.Unmarshal([]byte(data), &labelMap); err != nil {
			return result, err
		}
		for k, v := range labelMap {
			labels = append(labels, fmt.Sprintf("%s=%s", k, v))
		}
	}

	labels = append(labels, MachineUIDLabel+"="+string(entry.Machine.UID))

	sort.Strings(labels)
	config["node-label"] = labels

	if data := entry.Machine.Annotations[TaintsAnnotation]; data != "" {
		var (
			taints      []corev1.Taint
			taintString []string
		)
		if err := json.Unmarshal([]byte(data), &taints); err != nil {
			return result, err
		}
		for _, taint := range taints {
			taintString = append(taintString, fmt.Sprintf("%s=%s:%s", taint.Key, taint.Value, taint.Effect))
		}

		sort.Strings(taintString)
		config["node-taint"] = taintString
	}

	result.Instructions = append(result.Instructions, instruction)

	configData, err := json.Marshal(config)
	if err != nil {
		return result, err
	}

	result.Files = append(result.Files, plan.File{
		Content: base64.StdEncoding.EncodeToString(configData),
		Path:    fmt.Sprintf("/etc/rancher/%s/config.yaml", p.getRuntime(cluster)),
	})

	return result, nil
}

func (p *Planner) getRuntime(cluster *rkev1.RKECluster) string {
	return "k3s"
}

func (p *Planner) loadClusterAgent(cluster *rkev1.RKECluster) ([]byte, error) {
	tokens, err := p.clusterRegistrationTokenCache.GetByIndex(clusterRegToken, cluster.Spec.ManagementClusterName)
	if err != nil {
		return nil, err
	}

	url, ca, err := p.kubeconfig.GetServerURLAndCA()
	if err != nil {
		return nil, err
	}

	if len(tokens) == 0 {
		return nil, fmt.Errorf("no cluster registration token found")
	}

	return DownloadClusterAgentYAML(p.ctx, url, ca, tokens[0].Status.Token, cluster.Spec.ManagementClusterName)
}

func (p *Planner) kubernetesVersionToImage(version string) string {
	return "docker.io/oats87/loltgz:install-k3s"
}

func isEtcd(machine *capi.Machine) bool {
	return machine.Labels[EtcdRoleLabel] == "true"
}

func isInitNode(machine *capi.Machine) bool {
	return machine.Labels[InitNodeLabel] == "true"
}

func none(machine *capi.Machine) bool {
	return false
}

func isControlPlane(machine *capi.Machine) bool {
	return machine.Labels[ControlPlaneRoleLabel] == "true"
}

func isOnlyEtcd(machine *capi.Machine) bool {
	return isEtcd(machine) && !isControlPlane(machine)
}

func isOnlyWorker(machine *capi.Machine) bool {
	return !isEtcd(machine) && !isControlPlane(machine)
}

type planEntry struct {
	Machine *capi.Machine
	Plan    *plan.Node
}

func collect(plan *plan.Plan, include, exclude func(*capi.Machine) bool) (result []planEntry, unavailable int) {
	for name, machine := range plan.Machines {
		if !include(machine) || exclude(machine) {
			continue
		}
		result = append(result, planEntry{
			Machine: machine,
			Plan:    plan.Nodes[name],
		})
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Plan != nil && !result[i].Plan.InSync {
			unavailable++
		}
		return result[i].Machine.Name < result[j].Machine.Name
	})

	return result, unavailable
}

func (p *Planner) generateSecrets(cluster *rkev1.RKECluster) (*rkev1.RKECluster, plan.Secret, error) {
	secretName, secret, err := p.ensureRKEStateSecret(cluster)
	if err != nil {
		return nil, secret, err
	}

	cluster = cluster.DeepCopy()
	cluster.Status.ClusterStateSecretName = secretName
	return cluster, secret, nil
}

func (p *Planner) ensureRKEStateSecret(obj *rkev1.RKECluster) (string, plan.Secret, error) {
	name := name.SafeConcatName(obj.Name, "rke", "state")
	secret, err := p.secretCache.Get(obj.Namespace, name)
	if apierror.IsNotFound(err) {
		serverToken, err := randomtoken.Generate()
		if err != nil {
			return "", plan.Secret{}, err
		}

		agentToken, err := randomtoken.Generate()
		if err != nil {
			return "", plan.Secret{}, err
		}

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: obj.Namespace,
			},
			Data: map[string][]byte{
				"serverToken": []byte(serverToken),
				"agentToken":  []byte(agentToken),
			},
			Type: "rke.cattle.io/cluster-state",
		}

		_, err = p.secretClient.Create(secret)
		return name, plan.Secret{
			ServerToken: serverToken,
			AgentToken:  agentToken,
		}, err
	} else if err != nil {
		return "", plan.Secret{}, err
	}

	return secret.Name, plan.Secret{
		ServerToken: string(secret.Data["serverToken"]),
		AgentToken:  string(secret.Data["agentToken"]),
	}, nil
}
