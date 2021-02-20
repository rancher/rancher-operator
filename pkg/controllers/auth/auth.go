package auth

import (
	"context"

	v1 "github.com/rancher/rancher-operator/pkg/apis/rancher.cattle.io/v1"
	"github.com/rancher/rancher-operator/pkg/clients"
	projects2 "github.com/rancher/rancher-operator/pkg/controllers/projects"
	rocontrollers "github.com/rancher/rancher-operator/pkg/generated/controllers/rancher.cattle.io/v1"
	"github.com/rancher/rancher-operator/pkg/principals"
	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"github.com/rancher/wrangler/pkg/name"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
)

type handler struct {
	lookup   *principals.Lookup
	clusters rocontrollers.ClusterCache
	projects rocontrollers.ProjectCache
}

func Register(ctx context.Context, clients *clients.Clients, lookup *principals.Lookup) {
	h := handler{
		clusters: clients.Cluster.Cluster().Cache(),
		projects: clients.Cluster.Project().Cache(),
		lookup:   lookup,
	}

	rocontrollers.RegisterRoleTemplateBindingGeneratingHandler(ctx,
		clients.Cluster.RoleTemplateBinding(),
		clients.Apply.
			WithCacheTypes(clients.Management.ClusterRoleTemplateBinding(),
				clients.Management.ProjectRoleTemplateBinding()),
		"",
		"role-template-binding",
		h.onRoleTemplateBinding,
		nil)
}

func (h *handler) onRoleTemplateBinding(rtb *v1.RoleTemplateBinding, status v1.RoleTemplateBindingStatus) ([]runtime.Object, v1.RoleTemplateBindingStatus, error) {
	if rtb.BindingScope.APIGroup != "rancher.cattle.io" ||
		rtb.BindingScope.Selector == nil ||
		rtb.RoleTemplateName == "" {
		return nil, status, nil
	}

	sel, err := metav1.LabelSelectorAsSelector(rtb.BindingScope.Selector)
	if err != nil {
		return nil, status, err
	}

	switch rtb.BindingScope.Kind {
	case "Project":
		obj, err := h.onProjectRTB(rtb, sel)
		return obj, status, err
	case "Cluster":
		obj, err := h.onClusterRTB(rtb, sel)
		return obj, status, err
	}

	return nil, status, nil
}

func (h *handler) onProjectRTB(rtb *v1.RoleTemplateBinding, sel labels.Selector) ([]runtime.Object, error) {
	projects, err := h.projects.List(rtb.Namespace, sel)
	if err != nil {
		return nil, err
	}

	var result []runtime.Object

	for _, project := range projects {
		projects, err := projects2.Projects(project, h.clusters)
		if err != nil {
			return nil, err
		}
		for _, project := range projects {
			for _, subject := range rtb.Subjects {
				var (
					err  error
					crtb = &v3.ProjectRoleTemplateBinding{
						ObjectMeta: metav1.ObjectMeta{
							Name:      name.SafeConcatName(rtb.Name, "binding"),
							Namespace: project.Name,
						},
						ProjectName:      project.ClusterName + ":" + project.Name,
						RoleTemplateName: rtb.RoleTemplateName,
					}
				)

				if subject.Kind == "User" {
					crtb.UserPrincipalName, err = h.lookup.LookupUser(subject.Name)
				} else if subject.Kind == "Group" {
					crtb.GroupPrincipalName, err = h.lookup.LookupGroup(subject.Name)
				}

				if err != nil {
					return nil, err
				}

				result = append(result, crtb)
			}
		}
	}

	return result, nil
}

func (h *handler) onClusterRTB(rtb *v1.RoleTemplateBinding, sel labels.Selector) ([]runtime.Object, error) {
	clusters, err := h.clusters.List(rtb.Namespace, sel)
	if err != nil {
		return nil, err
	}

	var result []runtime.Object

	for _, cluster := range clusters {
		for _, subject := range rtb.Subjects {
			var (
				err  error
				crtb = &v3.ClusterRoleTemplateBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name.SafeConcatName(rtb.Name, "binding"),
						Namespace: cluster.Status.ClusterName,
					},
					ClusterName:      cluster.Status.ClusterName,
					RoleTemplateName: rtb.RoleTemplateName,
				}
			)

			if subject.Kind == "User" {
				crtb.UserPrincipalName, err = h.lookup.LookupUser(subject.Name)
			} else if subject.Kind == "Group" {
				crtb.GroupPrincipalName, err = h.lookup.LookupGroup(subject.Name)
			}

			if err != nil {
				return nil, err
			}

			result = append(result, crtb)
		}
	}

	return result, nil
}
