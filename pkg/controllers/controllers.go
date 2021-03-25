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
	cluster2 "github.com/rancher/rancher-operator/pkg/controllers/rke/cluster"
	"github.com/rancher/rancher-operator/pkg/controllers/rke/machine"
	machine_provision "github.com/rancher/rancher-operator/pkg/controllers/rke/machine-provision"
	node_reporter "github.com/rancher/rancher-operator/pkg/controllers/rke/node-reporter"
	"github.com/rancher/rancher-operator/pkg/controllers/rke/planner"
	"github.com/rancher/rancher-operator/pkg/controllers/rke/planstatus"
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
		cluster2.Register(ctx, clients)
		machine.Register(ctx, clients)
		machine_provision.Register(ctx, clients)
		planner.Register(ctx, clients)
		node_reporter.Register(ctx, clients)
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
