package v1

import (
	eksv1 "github.com/rancher/eks-operator/pkg/apis/eks.cattle.io/v1"
	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	rketypes "github.com/rancher/rke/types"
	"github.com/rancher/wrangler/pkg/genericcondition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Cluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ClusterSpec   `json:"spec"`
	Status            ClusterStatus `json:"status,omitempty"`
}

type ClusterSpec struct {
	FleetWorkspaceName            string                                  `json:"fleetWorkspaceName,omitempty"`
	ControlPlaneEndpoint          *Endpoint                               `json:"controlPlaneEndpoint,omitempty"`
	EKSConfig                     *eksv1.EKSClusterConfigSpec             `json:"eksConfig,omitempty"`
	ImportedConfig                *ImportedConfig                         `json:"importedConfig,omitempty"`
	ReferencedConfig              *ReferencedConfig                       `json:"referencedConfig,omitempty"`
	K3SConfig                     *v3.K3sConfig                           `json:"k3sConfig,omitempty"`
	LocalClusterAuthEndpoint      v3.LocalClusterAuthEndpoint             `json:"localClusterAuthEndpoint,omitempty"`
	RancherKubernetesEngineConfig *rketypes.RancherKubernetesEngineConfig `json:"rancherKubernetesEngineConfig,omitempty"`
	RKE2Config                    *v3.Rke2Config                          `json:"rke2Config,omitempty"`
}

type ClusterStatus struct {
	ClusterName        string                              `json:"clusterName,omitempty"`
	ClientSecretName   string                              `json:"clientSecretName,omitempty"`
	AgentDeployed      bool                                `json:"agentDeployed,omitempty"`
	ObservedGeneration int64                               `json:"observedGeneration"`
	Conditions         []genericcondition.GenericCondition `json:"conditions,omitempty"`
	Ready              bool                                `json:"ready,omitempty"`
}

type ImportedConfig struct {
	KubeConfigSecret string `json:"kubeConfigSecret,omitempty"`
}

type ReferencedConfig struct {
	Selector *metav1.LabelSelector `json:"selector,omitempty"`
}

type Endpoint struct {
	Host string `json:"host,omitempty"`
	Port int    `json:"port,omitempty"`
}
