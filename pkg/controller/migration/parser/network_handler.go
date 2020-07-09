package parser

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	operatorv1 "github.com/tigera/operator/pkg/apis/operator/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func handleNetwork(c *components, cfg *Config) error {
	// CALICO_NETWORKING_BACKEND
	netBackend, err := c.node.getEnv(ctx, c.client, "calico-node", "CALICO_NETWORKING_BACKEND")
	if err != nil {
		return err
	}
	if netBackend != nil && *netBackend != "" && *netBackend != "bird" {
		return ErrIncompatibleCluster{"only CALICO_NETWORKING_BACKEND=bird is supported at this time"}
	}

	// FELIX_DEFAULTENDPOINTTOHOSTACTION
	defaultWepAction, err := c.node.getEnv(ctx, c.client, "calico-node", "FELIX_DEFAULTENDPOINTTOHOSTACTION")
	if err != nil {
		return err
	}
	if defaultWepAction != nil && strings.ToLower(*defaultWepAction) != "accept" {
		return ErrIncompatibleCluster{
			fmt.Sprintf("unexpected FELIX_DEFAULTENDPOINTTOHOSTACTION: '%s'. Only 'accept' is supported.", *defaultWepAction),
		}
	}

	ipMethod, err := c.node.getEnv(ctx, c.client, "calico-node", "IP")
	if err != nil {
		return err
	}
	if ipMethod != nil && strings.ToLower(*ipMethod) != "autodetect" {
		return ErrIncompatibleCluster{
			fmt.Sprintf("unexpected IP value: '%s'. Only 'autodetect' is supported.", *ipMethod),
		}
	}

	// am, err := getEnvVar(ctx, c.client, node.Env, "IP_AUTODETECTION_METHOD")
	// if err != nil {
	// 	return err
	// }
	// tam, err := getAutoDetection(am)
	// if err != nil {
	// 	return err
	// }
	// config.AutoDetectionMethod = &tam

	// case "CALICO_IPV4POOL_IPIP", "CALICO_IPV4POOL_VXLAN":
	// 	// TODO
	// 	checkedVars[v.Name] = true

	cniConfig, err := c.node.getEnv(ctx, c.client, "install-cni", "CNI_NETWORK_CONFIG")
	if err != nil {
		return err
	}
	if cniConfig != nil {
		var cni map[string]interface{}
		bits := []byte(*cniConfig)
		if err := json.Unmarshal(bits, &cni); err != nil {
			return err
		}
	}

	mtu, err := c.node.getEnv(ctx, c.client, "install-cni", "CNI_MTU")
	if err != nil {
		return err
	}
	if mtu != nil {
		// TODO: dear god clean this up what is wrong with you
		i := intstr.FromString(*mtu)
		iv := int32(i.IntValue())
		cfg.Spec.CalicoNetwork = &operatorv1.CalicoNetworkSpec{
			MTU: &iv,
		}
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