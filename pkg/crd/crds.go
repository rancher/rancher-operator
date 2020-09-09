package crd

import (
	"context"

	v1 "github.com/rancher/rancher-operator/pkg/apis/rancher.cattle.io/v1"
	"github.com/rancher/wrangler/pkg/crd"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

func List() []crd.CRD {
	return []crd.CRD{
		newCRD(&v1.Cluster{}, func(c crd.CRD) crd.CRD {
			return c.
				WithColumn("Ready", ".status.ready").
				WithColumn("Kubeconfig", ".status.clientSecretName")
		}),
		newCRD(&v1.Project{}, func(c crd.CRD) crd.CRD {
			return c.
				WithColumn("Selector", ".spec.clusterSelector")
		}),
		newCRD(&v1.RoleTemplate{}, func(c crd.CRD) crd.CRD {
			c.NonNamespace = true
			return c
		}),
		newCRD(&v1.RoleTemplateBinding{}, func(c crd.CRD) crd.CRD {
			return c.
				WithColumn("Role", ".spec.roleTemplateName")
		}),
	}
}

func newCRD(obj interface{}, customize func(crd.CRD) crd.CRD) crd.CRD {
	crd := crd.CRD{
		GVK: schema.GroupVersionKind{
			Group:   "rancher.cattle.io",
			Version: "v1",
		},
		Status:       true,
		SchemaObject: obj,
	}
	if customize != nil {
		crd = customize(crd)
	}
	return crd
}

func WriteFile(filename string) error {
	return crd.WriteFile(filename, List())
}

func Create(ctx context.Context, cfg *rest.Config) error {
	return crd.Create(ctx, cfg, List())
}
