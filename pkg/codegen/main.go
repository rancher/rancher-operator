package main

import (
	"os"

	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"

	controllergen "github.com/rancher/wrangler/pkg/controller-gen"
	"github.com/rancher/wrangler/pkg/controller-gen/args"
)

func main() {
	os.Unsetenv("GOPATH")
	controllergen.Run(args.Options{
		OutputPackage: "github.com/rancher/rancher-operator/pkg/generated",
		Boilerplate:   "scripts/boilerplate.go.txt",
		Groups: map[string]args.Group{
			"rancher.cattle.io": {
				Types: []interface{}{
					"./pkg/apis/rancher.cattle.io/v1",
				},
				GenerateTypes: true,
			},
			"management.cattle.io": {
				Types: []interface{}{
					v3.Cluster{},
					v3.ClusterRegistrationToken{},
					v3.ClusterRoleTemplateBinding{},
					v3.Project{},
					v3.ProjectRoleTemplateBinding{},
					v3.RoleTemplate{},
					v3.Setting{},
					v3.Token{},
					v3.User{},
				},
			},
		},
	})
}
