package v1

import (
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type RoleTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Rules  []rbacv1.PolicyRule `json:"rules,omitempty"`
	Status RoleTemplateStatus  `json:"status,omitempty"`
}

type RoleTemplateStatus struct {
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type RoleTemplateBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	RoleTemplateName string                   `json:"roleTemplateName,omitempty"`
	BindingScope     RoleTemplateBindingScope `json:"bindingScope,omitempty"`
	// Subjects of only kind User/Group and apiGroup rancher.cattle.io are supported
	Subjects []rbacv1.Subject `json:"subjects,omitempty"`

	Status RoleTemplateBindingStatus `json:"status,omitempty"`
}

type RoleTemplateBindingStatus struct {
}

type RoleTemplateBindingScope struct {
	Selector *metav1.LabelSelector `json:"selector,omitempty"`
	// Kind only Cluster and Project are supported
	Kind string `json:"kind,omitempty"`
	// APIGroup only rancher.cattle.io is supported
	APIGroup string `json:"apiGroup,omitempty"`
}
