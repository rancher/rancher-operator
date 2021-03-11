module github.com/rancher/rancher-operator

go 1.15

replace k8s.io/client-go => k8s.io/client-go v0.20.2

require (
	github.com/evanphx/json-patch v4.9.0+incompatible
	github.com/go-logr/logr v0.3.0
	github.com/rancher/fleet/pkg/apis v0.0.0-20210203165831-44af1553b47e
	github.com/rancher/lasso v0.0.0-20210219163000-fcdfcec12969
	github.com/rancher/lasso/controller-runtime v0.0.0-20210219163000-fcdfcec12969
	github.com/rancher/norman v0.0.0-20210219160735-4b01fa823545
	github.com/rancher/rancher/pkg/apis v0.0.0-20210219041911-44129f287e3c
	github.com/rancher/rancher/pkg/client v0.0.0-20210219041911-44129f287e3c
	github.com/rancher/wrangler v0.7.3-0.20210220051046-ee3e0fff1d40
	github.com/sirupsen/logrus v1.6.0
	github.com/urfave/cli v1.22.2
	k8s.io/api v0.20.2
	k8s.io/apiextensions-apiserver v0.20.2
	k8s.io/apimachinery v0.20.2
	k8s.io/client-go v12.0.0+incompatible
	sigs.k8s.io/cluster-api v0.0.0
	sigs.k8s.io/controller-runtime v0.8.2
)

replace sigs.k8s.io/cluster-api => github.com/rancher/cluster-api v0.3.11-0.20210219162658-745452a60720
