package parser

import (
	"encoding/json"
	"strings"

	"github.com/containernetworking/cni/libcni"
)

// loadCNI loads CNI config into the components for all other handlers to use.
func loadCNI(c *components) error {
	cniConfig, err := c.node.getEnv(ctx, c.client, containerInstallCNI, "CNI_NETWORK_CONFIG")
	if err != nil {
		return err
	}
	if cniConfig == nil {
		return nil
	}

	conflist, err := loadCNIConfig(*cniConfig)
	if err != nil {
		return err
	}

	// convert to a map for simpler checks
	plugins := map[string]*libcni.NetworkConfig{}
	for _, plugin := range conflist.Plugins {
		if plugin.Network.Name == "calico" {
			json.Unmarshal(plugin.Bytes, c.calicoCNIConfig)
		} else {
			plugins[plugin.Network.Name] = plugin
		}
	}

	return nil
}

func loadCNIConfig(cniConfig string) (*libcni.NetworkConfigList, error) {
	// template out __CNI_MTU__ because it's a templated integer and will otherwise fail :(
	if strings.Contains(cniConfig, "__CNI_MTU__") {
		cniConfig = strings.Replace(cniConfig, "__CNI_MTU__", "-1", -1)
	}

	confList, err := libcni.ConfListFromBytes([]byte(cniConfig))
	if err == nil {
		return confList, nil
	}

	// if an error occured, try parsing it as a single item
	conf, err := libcni.ConfFromBytes([]byte(cniConfig))
	if err != nil {
		return nil, err
	}

	return libcni.ConfListFromConf(conf)
}
