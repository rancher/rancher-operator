package server

import (
	"context"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rancher/rancher-operator/pkg/clients"
	"github.com/rancher/rancher-operator/pkg/configserver"
	"github.com/rancher/rancher-operator/pkg/controllers/rke/machine"
	mgmtcontrollers "github.com/rancher/rancher-operator/pkg/generated/controllers/management.cattle.io/v3"
	"github.com/rancher/steve/pkg/aggregation"
)

func Register(ctx context.Context, systemNamespace string, clients *clients.Clients) {
	configServer := configserver.New(clients)

	router := mux.NewRouter()
	router.Handle("/v3/connect/agent", configServer)
	router.Handle("/system-agent-install.sh", InstallHandler(clients.Management.Setting().Cache()))

	aggregation.Watch(ctx,
		clients.Core.Secret(),
		systemNamespace,
		"steve-aggregation",
		router)
}

func InstallHandler(settings mgmtcontrollers.SettingCache) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		content, err := machine.InstallScript(settings)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}
		rw.Header().Set("Content-Type", "text/plain")
		rw.Write(content)
	})
}
