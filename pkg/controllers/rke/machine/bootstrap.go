package machine

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	mgmtcontroller "github.com/rancher/rancher-operator/pkg/generated/controllers/management.cattle.io/v3"
	"github.com/rancher/rancher-operator/pkg/settings"
)

func Bootstrap(settingsCache mgmtcontroller.SettingCache, token string) ([]byte, error) {
	url, err := settings.Get(settingsCache, "agent-install-script")
	if err != nil {
		return nil, err
	}
	script, err := ioutil.ReadFile(url)
	if os.IsNotExist(err) {
		resp, httpErr := http.Get(url)
		if httpErr != nil {
			return nil, httpErr
		}
		defer resp.Body.Close()
		script, err = ioutil.ReadAll(resp.Body)
	}
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
