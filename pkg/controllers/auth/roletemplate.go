package auth

import (
	"context"

	v1 "github.com/rancher/rancher-operator/pkg/apis/rancher.cattle.io/v1"
	"github.com/rancher/rancher-operator/pkg/clients"
	v12 "github.com/rancher/rancher-operator/pkg/generated/controllers/rancher.cattle.io/v1"
	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"github.com/rancher/wrangler/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func RegisterRoleTemplate(ctx context.Context, clients *clients.Clients) {
	v12.RegisterRoleTemplateGeneratingHandler(ctx,
		clients.Cluster.RoleTemplate(),
		clients.Apply.
			WithCacheTypes(clients.Management.RoleTemplate()),
		"",
		"role-template",
		onRoleTemplateChange,
		&generic.GeneratingHandlerOptions{
			AllowClusterScoped: true,
		})
}

func onRoleTemplateChange(rt *v1.RoleTemplate, status v1.RoleTemplateStatus) ([]runtime.Object, v1.RoleTemplateStatus, error) {
	return []runtime.Object{
		&v3.RoleTemplate{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name: rt.Name,
			},
			DisplayName: rt.Name,
			Description: rt.Annotations["field.cattle.io/description"],
			Rules:       rt.Rules,
		},
	}, status, nil
}
