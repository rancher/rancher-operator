package clients

import (
	"github.com/rancher/rancher-operator/pkg/generated/controllers/cluster.x-k8s.io"
	capicontrollers "github.com/rancher/rancher-operator/pkg/generated/controllers/cluster.x-k8s.io/v1alpha4"
	"github.com/rancher/rancher-operator/pkg/generated/controllers/fleet.cattle.io"
	fleetcontrollers "github.com/rancher/rancher-operator/pkg/generated/controllers/fleet.cattle.io/v1alpha1"
	"github.com/rancher/rancher-operator/pkg/generated/controllers/management.cattle.io"
	mgmtcontrollers "github.com/rancher/rancher-operator/pkg/generated/controllers/management.cattle.io/v3"
	"github.com/rancher/rancher-operator/pkg/generated/controllers/rancher.cattle.io"
	rocontrollers "github.com/rancher/rancher-operator/pkg/generated/controllers/rancher.cattle.io/v1"
	"github.com/rancher/rancher-operator/pkg/generated/controllers/rke.cattle.io"
	rkecontroller "github.com/rancher/rancher-operator/pkg/generated/controllers/rke.cattle.io/v1"
	"github.com/rancher/wrangler/pkg/clients"
	"k8s.io/client-go/tools/clientcmd"
)

type Clients struct {
	*clients.Clients

	Cluster    rocontrollers.Interface
	Management mgmtcontrollers.Interface
	CAPI       capicontrollers.Interface
	RKE        rkecontroller.Interface
	Fleet      fleetcontrollers.Interface
}

func New(clientConfig clientcmd.ClientConfig) (*Clients, error) {
	clients, err := clients.New(clientConfig, nil)
	if err != nil {
		return nil, err
	}

	opts := &fleet.FactoryOptions{
		SharedControllerFactory: clients.SharedControllerFactory,
	}

	return &Clients{
		Clients:    clients,
		CAPI:       cluster.NewFactoryFromConfigWithOptionsOrDie(clients.RESTConfig, opts).Cluster().V1alpha4(),
		RKE:        rke.NewFactoryFromConfigWithOptionsOrDie(clients.RESTConfig, opts).Rke().V1(),
		Cluster:    rancher.NewFactoryFromConfigWithOptionsOrDie(clients.RESTConfig, opts).Rancher().V1(),
		Management: management.NewFactoryFromConfigWithOptionsOrDie(clients.RESTConfig, opts).Management().V3(),
		Fleet:      fleet.NewFactoryFromConfigWithOptionsOrDie(clients.RESTConfig, opts).Fleet().V1alpha1(),
	}, nil
}
