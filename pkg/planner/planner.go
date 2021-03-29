package planner

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	rkev1 "github.com/rancher/rancher-operator/pkg/apis/rke.cattle.io/v1"
	"github.com/rancher/rancher-operator/pkg/apis/rke.cattle.io/v1/plan"
	"github.com/rancher/rancher-operator/pkg/clients"
	capicontrollers "github.com/rancher/rancher-operator/pkg/generated/controllers/cluster.x-k8s.io/v1alpha4"
	mgmtcontrollers "github.com/rancher/rancher-operator/pkg/generated/controllers/management.cattle.io/v3"
	"github.com/rancher/rancher-operator/pkg/kubeconfig"
	"github.com/rancher/rancher-operator/pkg/settings"
	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	corecontrollers "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/name"
	"github.com/rancher/wrangler/pkg/randomtoken"
	"github.com/rancher/wrangler/pkg/summary"
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

	RuntimeK3S  = "k3s"
	RuntimeRKE2 = "rke2"

	SecretTypeMachinePlan = "rke.cattle.io/machine-plan"
)

var (
	capiMachineLabel = "cluster.x-k8s.io/cluster-name"
)

type ErrWaiting string

func (e ErrWaiting) Error() string {
	return string(e)
}

type errIgnore string

func (e errIgnore) Error() string {
	return string(e)
}

type roleFilter func(machine *capi.Machine) bool

type Planner struct {
	ctx                           context.Context
	store                         *PlanStore
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
		store: NewStore(clients.Core.Secret(),
			clients.CAPI.Machine().Cache()),
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

func (p *Planner) Process(cluster *rkev1.RKECluster) error {
	plan, err := p.store.Load(cluster)
	if err != nil {
		return err
	}

	cluster, secret, err := p.generateSecrets(cluster)
	if err != nil {
		return err
	}

	if _, err := p.electInitNode(plan); err != nil {
		return err
	}

	var firstIgnoreError error

	err = p.reconcile(cluster, secret, plan, "bootstrap", isInitNode, none, cluster.Spec.UpgradeStrategy.ServerConcurrency, "")
	firstIgnoreError, err = ignoreErrors(firstIgnoreError, err)
	if err != nil {
		return err
	}

	joinServer, err := p.electInitNode(plan)
	if err != nil || joinServer == "" {
		return err
	}

	err = p.reconcile(cluster, secret, plan, "etcd", isEtcd, isInitNode, cluster.Spec.UpgradeStrategy.ServerConcurrency, joinServer)
	firstIgnoreError, err = ignoreErrors(firstIgnoreError, err)
	if err != nil {
		return err
	}

	err = p.reconcile(cluster, secret, plan, "control plane", isControlPlane, isInitNode, cluster.Spec.UpgradeStrategy.ServerConcurrency, joinServer)
	firstIgnoreError, err = ignoreErrors(firstIgnoreError, err)
	if err != nil {
		return err
	}

	err = p.reconcile(cluster, secret, plan, "worker", isOnlyWorker, isInitNode, cluster.Spec.UpgradeStrategy.WorkerConcurrency, joinServer)
	firstIgnoreError, err = ignoreErrors(firstIgnoreError, err)
	if err != nil {
		return err
	}

	if firstIgnoreError != nil {
		return ErrWaiting(firstIgnoreError.Error())
	}

	return nil
}

func ignoreErrors(firstIgnoreError error, err error) (error, error) {
	var errIgnore errIgnore
	if errors.As(err, &errIgnore) {
		if firstIgnoreError == nil {
			return err, nil
		}
		return firstIgnoreError, nil
	}
	return firstIgnoreError, err
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
	entries, _ := collect(plan, isEtcd)
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

func (p *Planner) reconcile(cluster *rkev1.RKECluster, secret plan.Secret, plan *plan.Plan,
	tierName string,
	include, exclude roleFilter, concurrency int, joinServer string) error {
	entries, unavailable := collect(plan, include)

	var (
		outOfSync   []string
		nonReady    []string
		errMachines []string
	)

	for _, entry := range entries {
		// we exclude here and not in collect to ensure that include matched at least on node
		if exclude(entry.Machine) {
			continue
		}

		summary := summary.Summarize(entry.Machine)
		if summary.Error {
			errMachines = append(errMachines, entry.Machine.Name)
		}
		if summary.Transitioning {
			nonReady = append(nonReady, entry.Machine.Name)
		}

		plan, err := p.desiredPlan(cluster, secret, entry, isInitNode(entry.Machine), joinServer)
		if err != nil {
			return err
		}

		if entry.Plan == nil {
			outOfSync = append(outOfSync, entry.Machine.Name)
			if err := p.store.UpdatePlan(entry.Machine, plan); err != nil {
				return err
			}
		} else if !equality.Semantic.DeepEqual(entry.Plan.Plan, plan) {
			outOfSync = append(outOfSync, entry.Machine.Name)
			if !entry.Plan.InSync || concurrency == 0 || unavailable < concurrency {
				if entry.Plan.InSync {
					unavailable++
				}
				if err := p.store.UpdatePlan(entry.Machine, plan); err != nil {
					return err
				}
			}
		} else if !entry.Plan.InSync {
			outOfSync = append(outOfSync, entry.Machine.Name)
		}
	}

	if len(entries) == 0 {
		return ErrWaiting("waiting for at least one " + tierName + " node")
	}

	errMachines = atMostThree(errMachines)
	if len(errMachines) > 0 {
		// we want these errors to get reported, but not block the process
		return errIgnore("failing " + tierName + " machine(s) " + strings.Join(errMachines, ","))
	}

	outOfSync = atMostThree(outOfSync)
	if len(outOfSync) > 0 {
		return ErrWaiting("provisioning " + tierName + " node(s) " + strings.Join(outOfSync, ","))
	}

	nonReady = atMostThree(nonReady)
	if len(nonReady) > 0 {
		// we want these errors to get reported, but not block the process
		return errIgnore("non-ready " + tierName + " machine(s) " + strings.Join(nonReady, ","))
	}

	return nil
}

func atMostThree(names []string) []string {
	if len(names) == 0 {
		return names
	}
	sort.Strings(names)
	if len(names) > 3 {
		names = names[:3]
	}
	return names
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
		if GetRuntime(cluster.Spec.KubernetesVersion) == RuntimeK3S {
			config["cluster-init"] = true
		}
	} else {
		// it's very important that the joinServer param isn't used on the initNode because that would
		// cause the plan to evaluate differently dependending on the arguments to desiredPlan, which
		// should alway return the same value for the same node regardless of other arguments
		config["server"] = joinServer
	}

	if isOnlyEtcd(entry.Machine) {
		config["role"] = "etcd"
	} else if isOnlyWorker(entry.Machine) {
		agent = true
	}

	runtime := GetRuntime(cluster.Spec.KubernetesVersion)

	if isControlPlane(entry.Machine) {
		data, err := p.loadClusterAgent(cluster)
		if err != nil {
			return result, err
		}
		result.Files = append(result.Files, plan.File{
			Content: base64.StdEncoding.EncodeToString(data),
			Path:    fmt.Sprintf("/var/lib/rancher/%s/server/manifests/cluster-agent.yaml", runtime),
		})
	}

	image, err := p.getInstallerImage(cluster)
	if err != nil {
		return result, err
	}

	instruction := plan.Instruction{
		Image:   image,
		Command: "sh",
		Args:    []string{"-c", "run.sh"},
	}

	if agent {
		instruction.Env = []string{
			fmt.Sprintf("INSTALL_%s_TYPE=agent", strings.ToUpper(runtime)),
		}
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

	configData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return result, err
	}

	result.Files = append(result.Files, plan.File{
		Content: base64.StdEncoding.EncodeToString(configData),
		Path:    fmt.Sprintf("/etc/rancher/%s/config.yaml", GetRuntime(cluster.Spec.KubernetesVersion)),
	})

	return result, nil
}

func GetRuntime(kubernetesVersion string) string {
	if strings.Contains(kubernetesVersion, RuntimeK3S) {
		return RuntimeK3S
	}
	return RuntimeRKE2
}

func (p *Planner) getInstallerImage(cluster *rkev1.RKECluster) (string, error) {
	if true {
		// The only working image right now
		return "docker.io/oats87/system-agent-installer-rke2:v1.19.8-alpha1-rke2r2", nil
	}

	runtime := GetRuntime(cluster.Spec.KubernetesVersion)
	image, err := settings.Get(p.settings, "system-agent-installer-image")
	if err != nil {
		return "", err
	}
	image = image + runtime + ":" + strings.ReplaceAll(cluster.Spec.KubernetesVersion, "+", "-")
	return settings.PrefixPrivateRegistry(p.settings, image)
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

func collect(plan *plan.Plan, include func(*capi.Machine) bool) (result []planEntry, unavailable int) {
	for name, machine := range plan.Machines {
		if !include(machine) {
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
