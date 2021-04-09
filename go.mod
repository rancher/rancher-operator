module github.com/rancher/rancher-operator

go 1.15

replace k8s.io/client-go => k8s.io/client-go v0.20.0

require (
	github.com/evanphx/json-patch v4.9.0+incompatible
	github.com/go-logr/logr v0.3.0 // indirect
	github.com/rancher/eks-operator v1.0.6
	github.com/rancher/fleet/pkg/apis v0.0.0-20210225010648-40ee92df4aea
	github.com/rancher/lasso v0.0.0-20200905045615-7fcb07d6a20b
	github.com/rancher/norman v0.0.0-20210225010917-c7fd1e24145b
	github.com/rancher/rancher/pkg/apis v0.0.0-20210409044207-9b0058c0329f
	github.com/rancher/rancher/pkg/client v0.0.0-20210409044207-9b0058c0329f
	github.com/rancher/rke v1.2.8-rc1.0.20210409005002-9213a4d08d08
	github.com/rancher/wrangler v0.7.3-0.20210331224822-5bd357588083
	github.com/sirupsen/logrus v1.6.0
	github.com/urfave/cli v1.22.2
	k8s.io/api v0.20.0
	k8s.io/apimachinery v0.20.0
	k8s.io/client-go v12.0.0+incompatible
)
