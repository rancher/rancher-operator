package planner

import (
	"context"
	"fmt"
	"strings"

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
