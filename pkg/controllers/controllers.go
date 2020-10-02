package controllers

import (
	"context"

	"github.com/rancher/rancher-operator/pkg/clients"
	"github.com/rancher/rancher-operator/pkg/controllers/auth"
	"github.com/rancher/rancher-operator/pkg/controllers/cluster"
	"github.com/rancher/rancher-operator/pkg/controllers/fleetcluster"
	"github.com/rancher/rancher-operator/pkg/controllers/projects"
	"github.com/rancher/rancher-operator/pkg/controllers/workspace"
	"github.com/rancher/rancher-operator/pkg/principals"
	"github.com/rancher/wrangler/pkg/leader"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/tools/clientcmd"
)

func Register(ctx context.Context, systemNamespace string, clientConfig clientcmd.ClientConfig) error {
	clients, err := clients.New(clientConfig)
	if err != nil {
		return err
	}

	lookup := principals.NewLookup(systemNamespace, "rancher-apikey", clients)

	cluster.Register(ctx, clients)
	projects.Register(ctx, clients)
	auth.Register(ctx, clients, lookup)
	auth.RegisterRoleTemplate(ctx, clients)
	workspace.Register(ctx, clients)
	fleetcluster.Register(ctx, clients)

	leader.RunOrDie(ctx, systemNamespace, "rancher-controller-lock", clients.K8s, func(ctx context.Context) {
		if err := clients.Start(ctx); err != nil {
			logrus.Fatal(err)
		}
		logrus.Info("All controllers are started")
	})

	return nil
}
