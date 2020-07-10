package parser

import (
	"encoding/json"
	"strings"

	"github.com/projectcalico/cni-plugin/pkg/types"
	v1 "github.com/tigera/operator/pkg/apis/operator/v1"

	"github.com/containernetworking/cni/libcni"
)

func handleCNI(c *components, cfg *Config) error {
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

	// check for calico plugin
	calicoConfig, ok := plugins["calico"]
	if !ok {
		return ErrIncompatibleCluster{"cni missing calico conf"}
	}

	var calicoConf types.NetConf
	if err := json.Unmarshal(calicoConfig.Bytes, &calicoConf); err != nil {
		return err
	}
	return processCNI(calicoConf)
}

func loadCNIConfig(cniConfig string) (*libcni.NetworkConfigList, error) {
	// template out __CNI_MTU__ because it's a templated integer and will otherwise fail :(
	cniConfig = strings.Replace(cniConfig, "__CNI_MTU__", "12345", -1)

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

func processCNI(conf types.NetConf) error {
	if len(conf.IPAM.IPv4Pools) != 0 {
		return ErrIncompatibleCluster{"cni ipam ranges not suported"}
	}
	if conf.FeatureControl.FloatingIPs {
		return ErrIncompatibleCluster{"floating IPs not supported"}
	}
	if conf.FeatureControl.IPAddrsNoIpam {
		return ErrIncompatibleCluster{"IpAddrsNoIpam not supported"}
	}

	return nil
}
