package clients

import (
	"context"

	"github.com/rancher/rancher-operator/pkg/crd"
	"github.com/rancher/rancher-operator/pkg/generated/controllers/management.cattle.io"
	mgmtcontrollers "github.com/rancher/rancher-operator/pkg/generated/controllers/management.cattle.io/v3"
	"github.com/rancher/rancher-operator/pkg/generated/controllers/rancher.cattle.io"
	rocontrollers "github.com/rancher/rancher-operator/pkg/generated/controllers/rancher.cattle.io/v1"
	"github.com/rancher/wrangler/pkg/apply"
	"github.com/rancher/wrangler/pkg/generated/controllers/apps"
	appcontrollers "github.com/rancher/wrangler/pkg/generated/controllers/apps/v1"
	"github.com/rancher/wrangler/pkg/generated/controllers/core"
	corecontrollers "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/generated/controllers/rbac"
	rbaccontrollers "github.com/rancher/wrangler/pkg/generated/controllers/rbac/v1"
	"github.com/rancher/wrangler/pkg/start"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type Clients struct {
	rocontrollers.Interface

	K8s        kubernetes.Interface
	Core       corecontrollers.Interface
	RBAC       rbaccontrollers.Interface
	Apps       appcontrollers.Interface
	Management mgmtcontrollers.Interface
	Apply      apply.Apply
	RESTConfig *rest.Config
	starters   []start.Starter
}

func (a *Clients) Start(ctx context.Context) error {
	if err := crd.Create(ctx, a.RESTConfig); err != nil {
		return err
	}

	return start.All(ctx, 5, a.starters...)
}

func New(cfg *rest.Config) (*Clients, error) {
	core, err := core.NewFactoryFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	corev := core.Core().V1()

	apps, err := apps.NewFactoryFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	appsv := apps.Apps().V1()

	rancher, err := rancher.NewFactoryFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	rancherv := rancher.Rancher().V1()

	mgmt, err := management.NewFactoryFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	mgmtv := mgmt.Management().V3()

	rbac, err := rbac.NewFactoryFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	rbacv := rbac.Rbac().V1()

	apply, err := apply.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	apply = apply.WithSetOwnerReference(false, false)

	k8s, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	return &Clients{
		K8s:        k8s,
		Interface:  rancherv,
		Core:       corev,
		RBAC:       rbacv,
		Apps:       appsv,
		Apply:      apply,
		Management: mgmtv,
		RESTConfig: cfg,
		starters: []start.Starter{
			core,
			rancher,
			mgmt,
			rbac,
			apps,
		},
	}, nil
}
