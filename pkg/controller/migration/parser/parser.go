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
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Config represents the configuration pulled from the existing install.
type Config struct {
	AutoDetectionMethod *operatorv1.NodeAddressAutodetection
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

	// get the calico-node container
	c := getContainer(ds.Spec.Template.Spec.Containers, "calico-node")
	if c == nil {
		return nil, fmt.Errorf("couldn't find calico-node container in existing calico-node daemonset")
	}

	// FELIX_DEFAULTENDPOINTTOHOSTACTION
	defaultWepAction := getEnv(c.Env, "FELIX_DEFAULTENDPOINTTOHOSTACTION")
	if defaultWepAction != nil && strings.ToLower(*defaultWepAction) != "accept" {
		return nil, ErrIncompatibleCluster{fmt.Sprintf("unexpected FELIX_DEFAULTENDPOINTTOHOSTACTION: '%s'. Only 'accept' is supported.", *defaultWepAction)}
	}

	// IP_AUTODETECTION_METHOD
	if am := getEnv(c.Env, "IP_AUTODETECTION_METHOD"); am != nil {
		tam, err := getAutoDetection(*am)
		if err != nil {
			return nil, err
		}
		config.AutoDetectionMethod = &tam
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
func getEnv(env []corev1.EnvVar, key string) *string {
	for _, e := range env {
		if e.Name == key {
			return &e.Value
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
