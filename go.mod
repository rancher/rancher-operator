module github.com/rancher/rancher-operator

go 1.14

replace k8s.io/client-go => github.com/rancher/client-go v0.18.8-fleet1

require (
	github.com/rancher/eks-operator v1.0.6-rc1
	github.com/rancher/fleet/pkg/apis v0.0.0-20200909045814-3675caaa7070
	github.com/rancher/lasso v0.0.0-20200905045615-7fcb07d6a20b
	github.com/rancher/norman v0.0.0-20200820172041-261460ee9088
	github.com/rancher/rancher/pkg/apis v0.0.0-20200915005652-d5ba6012d682
	github.com/rancher/rancher/pkg/client v0.0.0-20200915005652-d5ba6012d682
	github.com/rancher/rke v1.2.0-rc6
	github.com/rancher/wrangler v0.7.3-0.20201020003736-e86bc912dfac
	github.com/sirupsen/logrus v1.6.0
	github.com/urfave/cli v1.22.2
	k8s.io/api v0.18.8
	k8s.io/apimachinery v0.18.8
	k8s.io/client-go v12.0.0+incompatible
)
