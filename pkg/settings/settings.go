package settings

import (
	mgmtcontrollers "github.com/rancher/rancher-operator/pkg/generated/controllers/management.cattle.io/v3"
	apierror "k8s.io/apimachinery/pkg/api/errors"
)

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

func Bool(settings mgmtcontrollers.SettingCache, key string) (bool, error) {
	val, err := Get(settings, key)
	if apierror.IsNotFound(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return val == "true", nil
}

func Get(settings mgmtcontrollers.SettingCache, key string) (string, error) {
	server, err := settings.Get(key)
	if err != nil {
		return "", err
	}
	if server.Value == "" {
		return server.Default, nil
	}
	return server.Value, nil
}
