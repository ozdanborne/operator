// Package parser reads config from existing Calico installations that are not
// managed by Operator, and generates Operator Config that can be used
// to configure a similar cluster.
package parser

import (
	"context"
	"errors"
	"fmt"
	"strings"

	operatorv1 "github.com/tigera/operator/pkg/apis/operator/v1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Config represents the configuration pulled from the existing install.
type Config struct {
	AutoDetectionMethod *operatorv1.NodeAddressAutodetection
	MTU                 *int32
	FelixEnvVars        []corev1.EnvVar
}

// ErrIncompatibleCluster is thrown if a config option was detected in the existing install
// which Operator does not currently expose.
type ErrIncompatibleCluster struct {
	err string
}

func (e ErrIncompatibleCluster) Error() string {
	return e.err
}

func GetExistingConfig(ctx context.Context, client client.Client) (*Config, error) {
	config := &Config{}

	var ds = appsv1.DaemonSet{}
	err := client.Get(ctx, types.NamespacedName{
		Name:      "calico-node",
		Namespace: "kube-system",
	}, &ds)
	if err != nil {
		if kerrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	var checkedVars = map[string]bool{}

	// get the calico-node container
	c := getContainer(ds.Spec.Template.Spec.Containers, "calico-node")
	if c == nil {
		return nil, ErrIncompatibleCluster{"couldn't find calico-node container in existing calico-node daemonset"}
	}

	for _, v := range c.Env {
		if _, ok := checkedVars[v.Name]; ok {
			continue
		}

		switch v.Name {
		case "FELIX_DEFAULTENDPOINTTOHOSTACTION":
			defaultWepAction, err := getEnvVar(ctx, client, v)
			if err != nil {
				return nil, err
			}
			if strings.ToLower(defaultWepAction) != "accept" {
				return nil, ErrIncompatibleCluster{
					fmt.Sprintf("unexpected FELIX_DEFAULTENDPOINTTOHOSTACTION: '%s'. Only 'accept' is supported.", defaultWepAction),
				}
			}
			checkedVars[v.Name] = true

		case "IP":
			ipMethod, err := getEnvVar(ctx, client, v)
			if err != nil {
				return nil, err
			}
			if strings.ToLower(ipMethod) != "autodetect" {
				return nil, ErrIncompatibleCluster{
					fmt.Sprintf("unexpected IP value: '%s'. Only 'autodetect' is supported.", ipMethod),
				}
			}
			checkedVars[v.Name] = true

		case "IP_AUTODETECTION_METHOD":
			am, err := getEnvVar(ctx, client, v)
			if err != nil {
				return nil, err
			}
			tam, err := getAutoDetection(am)
			if err != nil {
				return nil, err
			}
			config.AutoDetectionMethod = &tam

			checkedVars[v.Name] = true

		case "CALICO_IPV4POOL_IPIP", "CALICO_IPV4POOL_VXLAN":
			// TODO
			checkedVars[v.Name] = true

		case "CALICO_NETWORKING_BACKEND":
			netBackend, err := getEnvVar(ctx, client, v)
			if err != nil {
				return nil, err
			}
			if netBackend != "bird" {
				return nil, ErrIncompatibleCluster{"only CALICO_NETWORKING_BACKEND=bird is supported at this time"}
			}

			checkedVars[v.Name] = true

		case "DATASTORE_TYPE":
			dsType, err := getEnvVar(ctx, client, v)
			if err != nil {
				return nil, err
			}
			if dsType != "kubernetes" {
				return nil, ErrIncompatibleCluster{"only CALICO_NETWORKING_BACKEND=bird is supported at this time"}
			}
			checkedVars["DATASTORE_TYPE"] = true

		// all ignored vars
		case "WAIT_FOR_DATASTORE",
			"CLUSTER_TYPE",
			"NODENAME",
			"CALICO_DISABLE_FILE_LOGGING":
			// ignore
			checkedVars[v.Name] = true

		// all validation vars
		default:
			if strings.HasPrefix(v.Name, "FELIX_") {
				config.FelixEnvVars = append(config.FelixEnvVars, v)
				checkedVars[v.Name] = true
				continue
			}
			return nil, ErrIncompatibleCluster{fmt.Sprintf("unexpected env var: %s", v.Name)}
		}
	}

	// go back through the list at the end to make sure we checked everything.
	for _, v := range c.Env {
		if _, ok := checkedVars[v.Name]; !ok {
			return nil, ErrIncompatibleCluster{fmt.Sprintf("unexpected env var: %s", v.Name)}
		}
	}

	// CNI_MTU
	cni := getContainer(ds.Spec.Template.Spec.InitContainers, "install-cni")
	if cni == nil {
		return nil, ErrIncompatibleCluster{"couldn't find install-cni container in existing calico-node daemonset"}
	}
	cniConfig, err := getEnv(ctx, client, cni.Env, "CNI_NETWORK_CONFIG")
	if err != nil {
		return nil, err
	}
	fmt.Println(*cniConfig)

	mtu, err := getEnv(ctx, client, cni.Env, "CNI_MTU")
	if err != nil {
		return nil, err
	}
	if mtu != nil {
		// TODO: dear god clean this up what is wrong with you
		i := intstr.FromString(*mtu)
		iv := int32(i.IntValue())
		config.MTU = &iv
	}

	return config, nil
}

func getContainer(containers []corev1.Container, name string) *corev1.Container {
	for _, container := range containers {
		if container.Name == name {
			return &container
		}
	}
	return nil
}

// getEnv gets an environment variable from a container. Nil is returned
// if the requested Key was not found.
func getEnv(ctx context.Context, client client.Client, env []corev1.EnvVar, key string) (*string, error) {
	for _, e := range env {
		if e.Name == key {
			val, err := getEnvVar(ctx, client, e)
			return &val, err
		}
	}
	return nil, nil
}

func getEnvVar(ctx context.Context, client client.Client, e corev1.EnvVar) (string, error) {
	if e.Value != "" {
		return e.Value, nil
	}
	// if Value is empty, one of the ConfigMapKeyRefs must be used
	if e.ValueFrom.ConfigMapKeyRef != nil {
		cm := v1.ConfigMap{}
		err := client.Get(ctx, types.NamespacedName{
			Name:      e.ValueFrom.ConfigMapKeyRef.LocalObjectReference.Name,
			Namespace: "kube-system",
		}, &cm)
		if err != nil {
			return "", err
		}
		v := cm.Data[e.ValueFrom.ConfigMapKeyRef.Key]
		return v, nil
	}

	// TODO: support secretRef, fieldRef, and resourceFieldRef
	return "", ErrIncompatibleCluster{"only configMapRef & explicit values supported for env vars at this time"}
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
