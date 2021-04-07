package planner

import (
	"context"
	"fmt"
	"sort"
	"strings"

	rkev1 "github.com/rancher/rancher-operator/pkg/apis/rke.cattle.io/v1"
	"github.com/rancher/rancher-operator/pkg/settings"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/rest"
)

func DownloadClusterAgentYAML(ctx context.Context, url, ca string, token, clusterID string) ([]byte, error) {
	client, err := rest.RESTClientFor(&rest.Config{
		ContentConfig: rest.ContentConfig{
			GroupVersion:         &schema.GroupVersion{},
			NegotiatedSerializer: &serializer.CodecFactory{},
		},
		Host: url,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: []byte(strings.TrimSpace(ca)),
		},
	})
	if err != nil {
		return nil, err
	}

	return client.Get().AbsPath("v3", "import", fmt.Sprintf("%v_%v.yaml", token, clusterID)).Do(ctx).Raw()
}

func (p *Planner) loadClusterAgent(controlPlane *rkev1.RKEControlPlane) ([]byte, error) {
	tokens, err := p.clusterRegistrationTokenCache.GetByIndex(clusterRegToken, controlPlane.Spec.ManagementClusterName)
	if err != nil {
		return nil, err
	}

	sort.Slice(tokens, func(i, j int) bool {
		return tokens[i].Name < tokens[j].Name
	})

	url, ca, err := settings.GetInternalServerURLAndCA(p.settings)
	if err != nil {
		return nil, err
	}

	if len(tokens) == 0 {
		return nil, fmt.Errorf("no cluster registration token found")
	}

	return DownloadClusterAgentYAML(p.ctx, url, ca, tokens[0].Status.Token, controlPlane.Spec.ManagementClusterName)
}
