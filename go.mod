module github.com/rancher/rancher-operator

go 1.14

replace k8s.io/client-go => k8s.io/client-go v0.18.0

require (
	github.com/rancher/eks-operator v0.1.0-rc22
	github.com/rancher/lasso v0.0.0-20200820172840-0e4cc0ef5cb0
	github.com/rancher/norman v0.0.0-20200820172041-261460ee9088
	github.com/rancher/rancher/pkg/apis v0.0.0-20200829054819-8d9cedde931c
	github.com/rancher/rancher/pkg/client v0.0.0-20200829054819-8d9cedde931c
	github.com/rancher/rke v1.2.0-rc6
	github.com/rancher/wrangler v0.6.2-0.20200822010948-6d667521af49
	github.com/sirupsen/logrus v1.6.0
	github.com/urfave/cli v1.22.2
	k8s.io/api v0.18.8
	k8s.io/apimachinery v0.18.8
	k8s.io/client-go v12.0.0+incompatible
)
