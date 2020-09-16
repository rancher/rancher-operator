package clients

import (
	"context"

	"github.com/rancher/rancher-operator/pkg/crd"
	"github.com/rancher/rancher-operator/pkg/generated/controllers/fleet.cattle.io"
	fleetcontrollers "github.com/rancher/rancher-operator/pkg/generated/controllers/fleet.cattle.io/v1alpha1"
	"github.com/rancher/rancher-operator/pkg/generated/controllers/management.cattle.io"
	mgmtcontrollers "github.com/rancher/rancher-operator/pkg/generated/controllers/management.cattle.io/v3"
	"github.com/rancher/rancher-operator/pkg/generated/controllers/rancher.cattle.io"
	rocontrollers "github.com/rancher/rancher-operator/pkg/generated/controllers/rancher.cattle.io/v1"
	"github.com/rancher/wrangler/pkg/clients"
	"github.com/rancher/wrangler/pkg/start"
	"k8s.io/client-go/tools/clientcmd"
)

type Clients struct {
	*clients.Clients
	rocontrollers.Interface

	Management mgmtcontrollers.Interface
	Fleet      fleetcontrollers.Interface

	starters []start.Starter
}

func (a *Clients) Start(ctx context.Context) error {
	if err := crd.Create(ctx, a.RESTConfig); err != nil {
		return err
	}

	if err := a.Clients.Start(ctx); err != nil {
		return err
	}

	return start.All(ctx, 5, a.starters...)
}

func New(clientConfig clientcmd.ClientConfig) (*Clients, error) {
	clients, err := clients.New(clientConfig, nil)
	if err != nil {
		return nil, err
	}

	rancher, err := rancher.NewFactoryFromConfig(clients.RESTConfig)
	if err != nil {
		return nil, err
	}

	mgmt, err := management.NewFactoryFromConfig(clients.RESTConfig)
	if err != nil {
		return nil, err
	}

	fleet, err := fleet.NewFactoryFromConfig(clients.RESTConfig)
	if err != nil {
		return nil, err
	}

	return &Clients{
		Clients:    clients,
		Interface:  rancher.Rancher().V1(),
		Management: mgmt.Management().V3(),
		Fleet:      fleet.Fleet().V1alpha1(),
		starters: []start.Starter{
			rancher,
			mgmt,
			fleet,
		},
	}, nil
}
