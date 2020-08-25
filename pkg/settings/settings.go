package settings

import mgmtcontrollers "github.com/rancher/rancher-operator/pkg/generated/controllers/management.cattle.io/v3"

func GetServerURLAndCA(settings mgmtcontrollers.SettingCache) (string, string, error) {
	server, err := settings.Get("server-url")
	if err != nil {
		return "", "", err
	}

	cacert, err := settings.Get("cacerts")
	if err != nil {
		return "", "", err
	}

	return server.Value, cacert.Value, nil
}
