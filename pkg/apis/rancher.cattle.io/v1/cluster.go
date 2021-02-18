package v1

import (
	eksv1 "github.com/rancher/eks-operator/pkg/apis/eks.cattle.io/v1"
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
	ControlPlaneEndpoint      *Endpoint                   `json:"controlPlaneEndpoint,omitempty"`
	EKSConfig                 *eksv1.EKSClusterConfigSpec `json:"eksConfig,omitempty"`
	ImportedConfig            *ImportedConfig             `json:"importedConfig,omitempty"`
	ReferencedConfig          *ReferencedConfig           `json:"referencedConfig,omitempty"`
	CloudCredentialSecretName string                      `json:"cloudCredentialSecretName,omitempty"`
	KubernetesVersion         string                      `json:"kubernetesVersion,omitempty"`
	RKEConfig                 *RKEConfig                  `json:"rkeConfig,omitempty"`
}

type ClusterStatus struct {
	Ready              bool                                `json:"ready,omitempty"`
	ClusterName        string                              `json:"clusterName,omitempty"`
	ClientSecretName   string                              `json:"clientSecretName,omitempty"`
	AgentDeployed      bool                                `json:"agentDeployed,omitempty"`
	ObservedGeneration int64                               `json:"observedGeneration"`
	Conditions         []genericcondition.GenericCondition `json:"conditions,omitempty"`
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
