package parser

import (
	"encoding/json"
	"strings"

	v1 "github.com/tigera/operator/pkg/apis/operator/v1"

	"github.com/containernetworking/cni/libcni"
)

// loadCNI loads CNI config into the components for all other handlers to use.
// while loading, it performs basic checks on other CNI plugins in the conflist, but
// stops at checking the calico conf, leaving that to the network handler.
func loadCNI(c *components, cfg *Config) error {
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
		plugins[plugin.Network.Name] = plugin
	}

	// check for portmap plugin
	if _, ok := plugins["portmap"]; ok {
		// why is this a const fjfjfiweljfiwoj
		hp := v1.HostPortsEnabled
		cfg.Spec.CalicoNetwork.HostPorts = &hp
	} else {
		hp := v1.HostPortsDisabled
		cfg.Spec.CalicoNetwork.HostPorts = &hp
	}

	// check for bandwidth plugin
	if _, ok := plugins["bandwidth"]; ok {
		return ErrIncompatibleCluster{"operator does not yet support bandwidth"}
	}

	calicoConfig, ok := plugins["calico"]
	if !ok {
		return ErrIncompatibleCluster{"operator does not yet support running without calico CNI"}
	}

	return json.Unmarshal(calicoConfig.Bytes, c.cniConfig)
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
