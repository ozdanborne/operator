// Package parser reads config from existing Calico installations that are not
// managed by Operator, and generates Operator Config that can be used
// to configure a similar cluster.
package parser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	operatorv1 "github.com/tigera/operator/pkg/apis/operator/v1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var ctx = context.Background()

// Config represents the configuration pulled from the existing install.
type Config struct {
	AutoDetectionMethod *operatorv1.NodeAddressAutodetection
	MTU                 *int32
	FelixEnvVars        []corev1.EnvVar
	CNIConfig           string
}

// ErrIncompatibleCluster is thrown if a config option was detected in the existing install
// which Operator does not currently expose.
type ErrIncompatibleCluster struct {
	err string
}

func (e ErrIncompatibleCluster) Error() string {
	return e.err
}

type RaemonSet struct {
	appsv1.DaemonSet

	checkedVars map[string]checkedFields
}

func (r *RaemonSet) uncheckedVars() []string {
	unchecked := []string{}

	for _, t := range r.Spec.Template.Spec.Containers {
		for _, v := range t.Env {

			if _, ok := r.checkedVars[t.Name].envVars[v.Name]; !ok {
				unchecked = append(unchecked, t.Name+"/"+v.Name)
			}
		}
	}
	return unchecked
}

// getEnv gets the value of an environment variable and marks that it has been checked.
func (r *RaemonSet) getEnv(ctx context.Context, client client.Client, container string, key string) (*string, error) {
	c := getContainers(r.Spec.Template.Spec, container)
	if c == nil {
		return nil, ErrIncompatibleCluster{fmt.Sprintf("couldn't find %s container in existing calico-node daemonset", container)}
	}
	r.ignoreEnv(container, key)
	return getEnv(ctx, client, c.Env, key)
}

func (r *RaemonSet) ignoreEnv(container, key string) {
	if _, ok := r.checkedVars[container]; !ok {
		r.checkedVars[container] = checkedFields{
			map[string]bool{},
		}
	}
	r.checkedVars[container].envVars[key] = true
}

type checkedFields struct {
	envVars map[string]bool
}

type components struct {
	// TODO: if we keep these as apimachinery structs, we can't
	// add custom fields to indicate if fields were checked.
	node            RaemonSet
	kubeControllers appsv1.Deployment
	typha           appsv1.Deployment
	client          client.Client
	checkedVars     map[string]bool
}

func getComponents(ctx context.Context, client client.Client) (*components, error) {
	var ds = appsv1.DaemonSet{}
	if err := client.Get(ctx, types.NamespacedName{
		Name:      "calico-node",
		Namespace: metav1.NamespaceSystem,
	}, &ds); err != nil {
		return nil, err
	}

	var kc = appsv1.Deployment{}
	if err := client.Get(ctx, types.NamespacedName{
		Name:      "calico-kube-controllers",
		Namespace: metav1.NamespaceSystem,
	}, &kc); err != nil {
		return nil, err
	}

	// TODO: handle partial detection
	// var t = appsv1.Deployment{}
	// if err := client.Get(ctx, types.NamespacedName{
	// 	Name:      "calico-typha",
	// 	Namespace: metav1.NamespaceSystem,
	// }, &t); err != nil {
	// 	return nil, err
	// }

	return &components{
		client: client,
		node: RaemonSet{
			ds,
			map[string]checkedFields{},
		},
		kubeControllers: kc,
		// typha:           t,

	}, nil
}

func (c *components) handleCore(*Config) error {
	dsType, err := c.node.getEnv(ctx, c.client, "calico-node", "DATASTORE_TYPE")
	if err != nil {
		return err
	}
	if dsType != nil && *dsType != "kubernetes" {
		return ErrIncompatibleCluster{"only CALICO_NETWORKING_BACKEND=bird is supported at this time"}
	}

	// mark other variables as ignored
	c.node.ignoreEnv("calico-node", "WAIT_FOR_DATASTORE")
	c.node.ignoreEnv("calico-node", "CLUSTER_TYPE")
	c.node.ignoreEnv("calico-node", "NODENAME")
	c.node.ignoreEnv("calico-node", "CALICO_DISABLE_FILE_LOGGING")

	return nil
}

func (c *components) handleNetwork(cfg *Config) error {
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
		cfg.MTU = &iv
	}

	return nil
}

func GetExistingConfig(ctx context.Context, client client.Client) (*Config, error) {
	config := &Config{}

	comps, err := getComponents(ctx, client)
	if err != nil {
		if kerrors.IsNotFound(err) {
			log.Print("no existing install found: ", err)
			return nil, nil
		}
		return nil, err
	}

	if err := comps.handleNetwork(config); err != nil {
		return nil, err
	}

	if err := comps.handleCore(config); err != nil {
		return nil, err
	}

	// uncheckedVars := comps.node.uncheckedVars()
	// // go back through the list at the end to make sure we checked everything.
	// if len(uncheckedVars) != 0 {
	// 	return nil, ErrIncompatibleCluster{fmt.Sprintf("unexpected env var: %s", uncheckedVars)}
	// }

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

func getContainers(spec corev1.PodSpec, name string) *corev1.Container {
	for _, container := range spec.Containers {
		if container.Name == name {
			return &container
		}
	}
	for _, container := range spec.InitContainers {
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
