package v1

import (
	"github.com/rancher/wrangler/pkg/genericcondition"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cluster-api/errors"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type RKEControlPlan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              RKEBootstrapSpec   `json:"spec"`
	Status            RKEBootstrapStatus `json:"status,omitempty"`
}

type RKEControlPlaneSpec struct {
	Replicas               *int32                 `json:"replicas,omitempty"`
	InfrastructureTemplate corev1.ObjectReference `json:"infrastructureTemplate"`
}

type RKEControlPlaneStatus struct {
	Ready              bool                                  `json:"ready"`
	FailureReason      errors.KubeadmControlPlaneStatusError `json:"failureReason,omitempty"`
	FailureMessage     *string                               `json:"failureMessage,omitempty"`
	ObservedGeneration int64                                 `json:"observedGeneration,omitempty"`
	Conditions         genericcondition.GenericCondition     `json:"conditions,omitempty"`
}
