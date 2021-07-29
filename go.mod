module github.com/rancher/rancher-operator

go 1.16

replace (
	k8s.io/client-go => k8s.io/client-go v0.20.2
	sigs.k8s.io/cluster-api => github.com/rancher/cluster-api v0.3.11-0.20210219162658-745452a60720
)

require (
	github.com/evanphx/json-patch v4.9.0+incompatible
	github.com/go-logr/logr v0.3.0
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/gorilla/mux v1.7.3
	github.com/rancher/fleet/pkg/apis v0.0.0-20210225010648-40ee92df4aea
	github.com/rancher/lasso v0.0.0-20210407025055-18a00567c734
	github.com/rancher/lasso/controller-runtime v0.0.0-20210219163000-fcdfcec12969
	github.com/rancher/norman v0.0.0-20210225010917-c7fd1e24145b
	github.com/rancher/rancher/pkg/apis v0.0.0-20210222182625-a85f4d1f87fe
	github.com/rancher/rancher/pkg/client v0.0.0-20210222182625-a85f4d1f87fe
	github.com/rancher/steve v0.0.0-20210318171316-376934558c5b
	github.com/rancher/wrangler v0.7.3-0.20210407025123-cf9bb4f55cee
	github.com/sirupsen/logrus v1.6.0
	github.com/urfave/cli v1.22.2
	k8s.io/api v0.20.2
	k8s.io/apiextensions-apiserver v0.20.2
	k8s.io/apimachinery v0.20.2
	k8s.io/client-go v12.0.0+incompatible
	sigs.k8s.io/cluster-api v0.0.0
	sigs.k8s.io/controller-runtime v0.8.2
)
