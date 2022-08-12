module github.com/rancher/rancher-operator

go 1.15

replace k8s.io/client-go => k8s.io/client-go v0.20.0

require (
	github.com/evanphx/json-patch v4.9.0+incompatible
	github.com/go-logr/logr v0.3.0 // indirect
	github.com/rancher/eks-operator v1.0.9
	github.com/rancher/fleet/pkg/apis v0.0.0-20210225010648-40ee92df4aea
	github.com/rancher/lasso v0.0.0-20220506212308-e3ef9b77cc49
	github.com/rancher/norman v0.0.0-20220712163932-620fef760449
	github.com/rancher/rancher/pkg/apis v0.0.0-20220811200500-ace7b2c56632
	github.com/rancher/rancher/pkg/client v0.0.0-20220811200500-ace7b2c56632
	github.com/rancher/rke v1.2.22-rc1
	github.com/rancher/wrangler v0.7.3-0.20210331224822-5bd357588083
	github.com/sirupsen/logrus v1.6.0
	github.com/urfave/cli v1.22.2
	k8s.io/api v0.20.0
	k8s.io/apimachinery v0.20.0
	k8s.io/client-go v12.0.0+incompatible
)
