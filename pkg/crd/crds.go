package crd

import (
	"context"
	"os"

	v1 "github.com/rancher/rancher-operator/pkg/apis/rancher.cattle.io/v1"
	rkev1 "github.com/rancher/rancher-operator/pkg/apis/rke.cattle.io/v1"
	"github.com/rancher/wrangler/pkg/crd"
	"github.com/rancher/wrangler/pkg/data"
	"github.com/rancher/wrangler/pkg/yaml"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

var (
	capiCRDs = map[string]bool{
		"Machine":            true,
		"MachineHealthCheck": true,
		"MachineDeployment":  true,
		"MachineSet":         true,
		"Cluster":            true,
	}
)

func List() []crd.CRD {
	return append(operator(), capi()...)
}

func operator() []crd.CRD {
	return []crd.CRD{
		newRancherCRD(&v1.Cluster{}, func(c crd.CRD) crd.CRD {
			return c.
				WithColumn("Ready", ".status.ready").
				WithColumn("Kubeconfig", ".status.clientSecretName")
		}),
		newRancherCRD(&v1.Project{}, func(c crd.CRD) crd.CRD {
			return c.
				WithColumn("Selector", ".spec.clusterSelector")
		}),
		newRancherCRD(&v1.RoleTemplate{}, func(c crd.CRD) crd.CRD {
			c.NonNamespace = true
			return c
		}),
		newRancherCRD(&v1.RoleTemplateBinding{}, func(c crd.CRD) crd.CRD {
			return c.
				WithColumn("Role", ".spec.roleTemplateName")
		}),
		newRKECRD(&rkev1.RKECluster{}, func(c crd.CRD) crd.CRD {
			c.Labels = map[string]string{
				"cluster.x-k8s.io/v1alpha4": "v1",
			}
			return c
		}),
		newRKECRD(&rkev1.RKEBootstrap{}, func(c crd.CRD) crd.CRD {
			c.Labels = map[string]string{
				"cluster.x-k8s.io/v1alpha4": "v1",
			}
			return c
		}),
		newRKECRD(&rkev1.RKEBootstrapTemplate{}, func(c crd.CRD) crd.CRD {
			c.Labels = map[string]string{
				"cluster.x-k8s.io/v1alpha4": "v1",
			}
			return c
		}),
		newRKECRD(&rkev1.RKEControlPlan{}, func(c crd.CRD) crd.CRD {
			c.Labels = map[string]string{
				"cluster.x-k8s.io/v1alpha4": "v1",
			}
			return c
		}),
		newRKECRD(&rkev1.UnmanagedMachine{}, func(c crd.CRD) crd.CRD {
			c.Labels = map[string]string{
				"cluster.x-k8s.io/v1alpha4": "v1",
			}
			return c
		}),
	}
}

func capi() []crd.CRD {
	f, err := os.Open("./scripts/capi-crds.yaml")
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		panic(err)
	}
	defer f.Close()

	objs, err := yaml.ToObjects(f)
	if err != nil {
		panic(err)
	}

	var result []crd.CRD
	for _, obj := range objs {
		if obj.GetObjectKind().GroupVersionKind().Kind != "CustomResourceDefinition" {
			continue
		}
		if unstr, ok := obj.(*unstructured.Unstructured); ok &&
			capiCRDs[data.Object(unstr.Object).String("spec", "names", "kind")] {
			result = append(result, crd.CRD{
				Override: obj,
			})
		}
	}

	return result
}

func newRKECRD(obj interface{}, customize func(crd.CRD) crd.CRD) crd.CRD {
	crd := crd.CRD{
		GVK: schema.GroupVersionKind{
			Group:   "rke.cattle.io",
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

func newRancherCRD(obj interface{}, customize func(crd.CRD) crd.CRD) crd.CRD {
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

func WriteFileCAPI(filename string) error {
	return crd.WriteFile(filename, capi())
}

func WriteFileOperator(filename string) error {
	return crd.WriteFile(filename, operator())
}

func Create(ctx context.Context, cfg *rest.Config) error {
	return crd.Create(ctx, cfg, List())
}
