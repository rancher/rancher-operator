package controllers

import (
	"context"

	"github.com/rancher/rancher-operator/pkg/capi"
	"github.com/rancher/rancher-operator/pkg/clients"
	"github.com/rancher/rancher-operator/pkg/controllers/auth"
	"github.com/rancher/rancher-operator/pkg/controllers/cluster"
	"github.com/rancher/rancher-operator/pkg/controllers/dynamicschema"
	"github.com/rancher/rancher-operator/pkg/controllers/fleetcluster"
	"github.com/rancher/rancher-operator/pkg/controllers/managerancher"
	"github.com/rancher/rancher-operator/pkg/controllers/projects"
	"github.com/rancher/rancher-operator/pkg/controllers/rke/bootstrap"
	machine_provision "github.com/rancher/rancher-operator/pkg/controllers/rke/machine-provision"
	"github.com/rancher/rancher-operator/pkg/controllers/rke/planner"
	"github.com/rancher/rancher-operator/pkg/controllers/rke/planstatus"
	"github.com/rancher/rancher-operator/pkg/controllers/rke/ranchercluster"
	"github.com/rancher/rancher-operator/pkg/controllers/rke/rkecluster"
	"github.com/rancher/rancher-operator/pkg/controllers/rke/unmanaged"
	"github.com/rancher/rancher-operator/pkg/controllers/workspace"
	"github.com/rancher/rancher-operator/pkg/crd"
	"github.com/rancher/rancher-operator/pkg/principals"
	"github.com/rancher/rancher-operator/pkg/server"
	"github.com/rancher/wrangler/pkg/leader"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/tools/clientcmd"
)

func Register(ctx context.Context, capiEnabled, rkeEnabled, crdEnabled bool, systemNamespace string, clientConfig clientcmd.ClientConfig) error {
	clients, err := clients.New(clientConfig)
	if err != nil {
		return err
	}

	if crdEnabled {
		if err := crd.Create(ctx, clients.RESTConfig); err != nil {
			return err
		}
	}

	lookup := principals.NewLookup("", "rancher-apikey", clients)

	cluster.Register(ctx, clients)
	projects.Register(ctx, clients)
	auth.Register(ctx, clients, lookup)
	auth.RegisterRoleTemplate(ctx, clients)
	workspace.Register(ctx, clients)
	fleetcluster.Register(ctx, clients)
	managerancher.Register(ctx, clients)

	if rkeEnabled {
		dynamicschema.Register(ctx, clients)
		rkecluster.Register(ctx, clients)
		ranchercluster.Register(ctx, clients)
		bootstrap.Register(ctx, clients)
		machine_provision.Register(ctx, clients)
		planner.Register(ctx, clients)
		planstatus.Register(ctx, clients)
		unmanaged.Register(ctx, clients)
		server.Register(ctx, systemNamespace, clients)
	}

	var capiStart func(context.Context) error
	if capiEnabled {
		capiStart, err = capi.Register(ctx, clients)
		if err != nil {
			return err
		}
	}

	leader.RunOrDie(ctx, "", "rancher-controller-lock", clients.K8s, func(ctx context.Context) {
		if err := clients.Start(ctx); err != nil {
			logrus.Fatal(err)
		}
		logrus.Info("All controllers are started")
		if capiStart != nil {
			if err := capiStart(ctx); err != nil {
				logrus.Fatal(err)
			}
			logrus.Info("Cluster API is started")
		}
	})

	return nil
}
