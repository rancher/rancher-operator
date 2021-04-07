package bootstrap

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	mgmtcontroller "github.com/rancher/rancher-operator/pkg/generated/controllers/management.cattle.io/v3"
	"github.com/rancher/rancher-operator/pkg/settings"
)

var (
	defaultSystemAgentInstallScript = "https://raw.githubusercontent.com/rancher/system-agent/main/install.sh"
	localAgentInstallScript         = "./install.sh"
)

func InstallScript(settingsCache mgmtcontroller.SettingCache) ([]byte, error) {
	url, err := settings.Get(settingsCache, "system-agent-install-script")
	if err != nil {
		return nil, err
	}

	if url == "" {
		script, err := ioutil.ReadFile(localAgentInstallScript)
		if !os.IsNotExist(err) {
			return script, err
		}
	}

	if url == "" {
		url = defaultSystemAgentInstallScript
	}

	resp, httpErr := http.Get(url)
	if httpErr != nil {
		return nil, httpErr
	}
	defer resp.Body.Close()

	return ioutil.ReadAll(resp.Body)
}

func Bootstrap(settingsCache mgmtcontroller.SettingCache, token string) ([]byte, error) {
	script, err := InstallScript(settingsCache)
	if err != nil {
		return nil, err
	}

	url, ca, err := settings.GetServerURLAndCAChecksum(settingsCache)
	if err != nil {
		return nil, err
	}

	return []byte(fmt.Sprintf(`#!/usr/bin/env sh
CATTLE_SERVER="%s"
CATTLE_CA_CHECKSUM="%s"
CATTLE_TOKEN="%s"

%s
`, url, ca, token, script)), nil
}
