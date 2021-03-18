//go:generate go run pkg/codegen/cleanup/main.go
//go:generate go run pkg/codegen/main.go
//go:generate go run main.go --write-crds ./charts/rancher-operator-crd/templates/crds.yaml
//go:generate go run main.go --write-capi-crds ./charts/rancher-operator-crd/charts/capi/templates/crds.yaml

package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/rancher/rancher-operator/pkg/controllers"
	"github.com/rancher/rancher-operator/pkg/crd"
	"github.com/rancher/wrangler/pkg/kubeconfig"
	"github.com/rancher/wrangler/pkg/signals"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"

	_ "github.com/rancher/wrangler/pkg/generated/controllers/apiextensions.k8s.io/v1beta1"
)

var (
	Version       = "v0.0.0-dev"
	GitCommit     = "HEAD"
	KubeConfig    string
	Context       string
	Namespace     string
	WriteCRDs     string
	WriteCAPICRDs string
	EnableCAPI    bool
	EnableRKE     bool
	SkipCRD       bool
)

func main() {
	app := cli.NewApp()
	app.Name = "rancher"
	app.Version = fmt.Sprintf("%s (%s)", Version, GitCommit)
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:        "kubeconfig",
			EnvVar:      "KUBECONFIG",
			Destination: &KubeConfig,
		},
		cli.StringFlag{
			Name:        "context",
			EnvVar:      "CONTEXT",
			Destination: &Context,
		},
		cli.StringFlag{
			Name:        "namespace",
			EnvVar:      "NAMESPACE",
			Destination: &Namespace,
			Value:       "rancher-operator-system",
		},
		cli.StringFlag{
			Name:        "write-crds",
			Destination: &WriteCRDs,
		},
		cli.StringFlag{
			Name:        "write-capi-crds",
			Destination: &WriteCAPICRDs,
		},
		cli.BoolFlag{
			Name:        "enable-capi",
			Destination: &EnableCAPI,
			EnvVar:      "ENABLE_CAPI",
		},
		cli.BoolFlag{
			Name:        "enable-rke",
			Destination: &EnableRKE,
			EnvVar:      "ENABLE_RKE",
		},
		cli.BoolFlag{
			Name:        "skip-crd",
			Destination: &SkipCRD,
			EnvVar:      "SKIP_CRDS",
		},
	}
	app.Action = run

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}

func run(c *cli.Context) error {
	rand.Seed(time.Now().UnixNano())

	if WriteCRDs != "" || WriteCAPICRDs != "" {
		if WriteCRDs != "" {
			logrus.Info("Writing CRDS to ", WriteCRDs)
			return crd.WriteFileOperator(WriteCRDs)
		}
		if WriteCAPICRDs != "" {
			logrus.Info("Writing CAPI CRDS to ", WriteCAPICRDs)
			return crd.WriteFileCAPI(WriteCAPICRDs)
		}
		return nil
	}

	logrus.Info("Starting controller")
	ctx := signals.SetupSignalHandler(context.Background())
	clientConfig := kubeconfig.GetNonInteractiveClientConfigWithContext(KubeConfig, Context)

	if err := controllers.Register(ctx, EnableCAPI, EnableRKE, !SkipCRD, Namespace, clientConfig); err != nil {
		return err
	}

	<-ctx.Done()
	return nil
}
