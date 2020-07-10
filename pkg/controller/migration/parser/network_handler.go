package parser

import (
	"errors"
	"fmt"
	"strings"

	operatorv1 "github.com/tigera/operator/pkg/apis/operator/v1"
	v1 "github.com/tigera/operator/pkg/apis/operator/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	containerCalicoNode = "calico-node"
	containerInstallCNI = "install-cni"
)

func handleNetwork(c *components, cfg *Config) error {
	// CALICO_NETWORKING_BACKEND
	netBackend, err := c.node.getEnv(ctx, c.client, containerCalicoNode, "CALICO_NETWORKING_BACKEND")
	if err != nil {
		return err
	}
	if netBackend != nil && *netBackend != "" && *netBackend != "bird" {
		return ErrIncompatibleCluster{"only CALICO_NETWORKING_BACKEND=bird is supported at this time"}
	}

	// FELIX_DEFAULTENDPOINTTOHOSTACTION
	defaultWepAction, err := c.node.getEnv(ctx, c.client, containerCalicoNode, "FELIX_DEFAULTENDPOINTTOHOSTACTION")
	if err != nil {
		return err
	}
	if defaultWepAction != nil && strings.ToLower(*defaultWepAction) != "accept" {
		return ErrIncompatibleCluster{
			fmt.Sprintf("unexpected FELIX_DEFAULTENDPOINTTOHOSTACTION: '%s'. Only 'accept' is supported.", *defaultWepAction),
		}
	}

	ipMethod, err := c.node.getEnv(ctx, c.client, containerCalicoNode, "IP")
	if err != nil {
		return err
	}
	if ipMethod != nil && strings.ToLower(*ipMethod) != "autodetect" {
		return ErrIncompatibleCluster{
			fmt.Sprintf("unexpected IP value: '%s'. Only 'autodetect' is supported.", *ipMethod),
		}
	}

	am, err := c.node.getEnv(ctx, c.client, containerCalicoNode, "IP_AUTODETECTION_METHOD")
	if err != nil {
		return err
	}
	if am != nil {
		tam, err := getAutoDetection(*am)
		if err != nil {
			return err
		}
		cfg.Spec.CalicoNetwork.NodeAddressAutodetectionV4 = &tam
	}

	// check for portmap plugin
	if _, ok := c.pluginCNIConfig["portmap"]; ok {
		// can't take address of const's so copy it into a new var oiwjfeoiwapcj;eifj
		hp := v1.HostPortsEnabled
		cfg.Spec.CalicoNetwork.HostPorts = &hp
	} else {
		hp := v1.HostPortsDisabled
		cfg.Spec.CalicoNetwork.HostPorts = &hp
	}

	// check for bandwidth plugin
	if _, ok := c.pluginCNIConfig["bandwidth"]; ok {
		return ErrIncompatibleCluster{"operator does not yet support bandwidth"}
	}

	if c.calicoCNIConfig == nil {
		// TODO: don't return an error once we support this, instead just returning nil.
		return ErrIncompatibleCluster{"operator does not yet support running without calico CNI"}
	}

	if c.calicoCNIConfig.MTU == -1 {
		// if MTU is -1, we can assume it was us who replaced it when doing initial CNI
		// config loading. We need to pull it from the correct source
		mtu, err := c.node.getEnv(ctx, c.client, containerInstallCNI, "CNI_MTU")
		if err != nil {
			return err
		}
		if mtu == nil {
			return fmt.Errorf("couldn't detect MTU")
		}

		// TODO: dear god clean this up what is wrong with you
		i := intstr.FromString(*mtu)
		iv := int32(i.IntValue())
		cfg.Spec.CalicoNetwork = &operatorv1.CalicoNetworkSpec{
			MTU: &iv,
		}
	} else {
		// user must have hardcoded their CNI instead of using our cni templating engine
		mtu := int32(c.calicoCNIConfig.MTU)
		cfg.Spec.CalicoNetwork.MTU = &mtu
	}

	// check other cni settings
	if len(c.calicoCNIConfig.IPAM.IPv4Pools) != 0 {
		return ErrIncompatibleCluster{"cni ipam ranges not suported"}
	}
	if c.calicoCNIConfig.FeatureControl.FloatingIPs {
		return ErrIncompatibleCluster{"floating IPs not supported"}
	}
	if c.calicoCNIConfig.FeatureControl.IPAddrsNoIpam {
		return ErrIncompatibleCluster{"IpAddrsNoIpam not supported"}
	}

	return nil
}

// autoDetectCIDR auto-detects the IP and Network using the requested
// detection method.
func getAutoDetection(method string) (operatorv1.NodeAddressAutodetection, error) {
	const (
		AUTODETECTION_METHOD_FIRST          = "first-found"
		AUTODETECTION_METHOD_CAN_REACH      = "can-reach="
		AUTODETECTION_METHOD_INTERFACE      = "interface="
		AUTODETECTION_METHOD_SKIP_INTERFACE = "skip-interface="
	)

	if method == "" || method == AUTODETECTION_METHOD_FIRST {
		// Autodetect the IP by enumerating all interfaces (excluding
		// known internal interfaces).
		var t = true
		return operatorv1.NodeAddressAutodetection{FirstFound: &t}, nil
	}

	// For 'interface', autodetect the IP from the specified interface.
	if strings.HasPrefix(method, AUTODETECTION_METHOD_INTERFACE) {
		ifStr := strings.TrimPrefix(method, AUTODETECTION_METHOD_INTERFACE)
		return operatorv1.NodeAddressAutodetection{Interface: ifStr}, nil
	}

	// For 'can-reach', autodetect the IP by connecting a UDP socket to a supplied address.
	if strings.HasPrefix(method, AUTODETECTION_METHOD_CAN_REACH) {
		dest := strings.TrimPrefix(method, AUTODETECTION_METHOD_CAN_REACH)
		return operatorv1.NodeAddressAutodetection{CanReach: dest}, nil
	}

	// For 'skip', autodetect the Ip by enumerating all interfaces (excluding
	// known internal interfaces and any interfaces whose name
	// matches the given regexes).
	if strings.HasPrefix(method, AUTODETECTION_METHOD_SKIP_INTERFACE) {
		ifStr := strings.TrimPrefix(method, AUTODETECTION_METHOD_SKIP_INTERFACE)
		return operatorv1.NodeAddressAutodetection{SkipInterface: ifStr}, nil
	}

	return operatorv1.NodeAddressAutodetection{}, errors.New("unrecognized option for AUTODETECTION_METHOD_SKIP_INTERFACE: " + method)
}
