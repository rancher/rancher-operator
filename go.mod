module github.com/rancher/rancher-operator

go 1.15

replace (
	github.com/rancher/rancher/pkg/apis => github.com/pennyscissors/rancher/pkg/apis v0.0.0-20210205233934-6d0193d42bf0
	github.com/rancher/rancher/pkg/client => github.com/pennyscissors/rancher/pkg/client v0.0.0-20210205233934-6d0193d42bf0
	github.com/rancher/rke => github.com/pennyscissors/rke v1.2.0-rc2.0.20210205224836-bc1782733189
	k8s.io/client-go => k8s.io/client-go v0.20.0
)

require (
	github.com/rancher/eks-operator v1.0.6-rc1
	github.com/rancher/fleet/pkg/apis v0.0.0-20210203165831-44af1553b47e
	github.com/rancher/lasso v0.0.0-20200905045615-7fcb07d6a20b
	github.com/rancher/norman v0.0.0-20200930000340-693d65aaffe3
	github.com/rancher/rancher/pkg/apis v0.0.0-20210203185325-55c67516629f
	github.com/rancher/rancher/pkg/client v0.0.0-20210203185325-55c67516629f
	github.com/rancher/rke v1.2.6-rc4.0.20210205231142-0e2db503c5a2
	github.com/rancher/wrangler v0.7.3-0.20201028210318-d73835950c29
	github.com/sirupsen/logrus v1.6.0
	github.com/urfave/cli v1.22.2
	k8s.io/api v0.20.0
	k8s.io/apimachinery v0.20.0
	k8s.io/client-go v12.0.0+incompatible
)
