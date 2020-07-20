package defaults

import (
	"context"
	"fmt"
	"strings"

	operatorv1 "github.com/tigera/operator/pkg/apis/operator/v1"
	"github.com/tigera/operator/pkg/render"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Convert(ctx context.Context, client client.Client, instance *operatorv1.Installation) error {
	// Populate the instance with defaults for any fields not provided by the user.
	if len(instance.Spec.Registry) != 0 && !strings.HasSuffix(instance.Spec.Registry, "/") {
		// Make sure registry always ends with a slash.
		instance.Spec.Registry = fmt.Sprintf("%s/", instance.Spec.Registry)
	}

	if len(instance.Spec.Variant) == 0 {
		// Default to installing Calico.
		instance.Spec.Variant = operatorv1.Calico
	}

	// Based on the Kubernetes provider, we may or may not need to default to using Calico networking.
	// For managed clouds, we use the cloud provided networking. For other platforms, use Calico networking.
	switch instance.Spec.KubernetesProvider {
	case operatorv1.ProviderAKS, operatorv1.ProviderEKS, operatorv1.ProviderGKE:
		if instance.Spec.CalicoNetwork != nil {
			// For these platforms, it's an error to have CalicoNetwork set.
			msg := "Installation spec.calicoNetwork must not be set for provider %s"
			return fmt.Errorf(msg, instance.Spec.KubernetesProvider)
		}
	default:
		if instance.Spec.CalicoNetwork == nil {
			// For all other platforms, default to using Calico networking.
			instance.Spec.CalicoNetwork = &operatorv1.CalicoNetworkSpec{}
		}
	}

	var v4pool, v6pool *operatorv1.IPPool

	// If Calico networking is in use, then default some fields.
	if instance.Spec.CalicoNetwork != nil {
		// Default IP pools, only if it is nil.
		// If it is an empty slice then that means no default IPPools
		// should be created.
		if instance.Spec.CalicoNetwork.IPPools == nil {
			instance.Spec.CalicoNetwork.IPPools = []operatorv1.IPPool{
				operatorv1.IPPool{CIDR: "192.168.0.0/16"},
			}
		}

		v4pool = render.GetIPv4Pool(instance.Spec.CalicoNetwork)
		v6pool = render.GetIPv6Pool(instance.Spec.CalicoNetwork)

		if v4pool != nil {
			if v4pool.Encapsulation == "" {
				v4pool.Encapsulation = operatorv1.EncapsulationDefault
			}
			if v4pool.NATOutgoing == "" {
				v4pool.NATOutgoing = operatorv1.NATOutgoingEnabled
			}
			if v4pool.NodeSelector == "" {
				v4pool.NodeSelector = operatorv1.NodeSelectorDefault
			}
			if instance.Spec.CalicoNetwork.NodeAddressAutodetectionV4 == nil {
				// Default IPv4 address detection to "first found" if not specified.
				t := true
				instance.Spec.CalicoNetwork.NodeAddressAutodetectionV4 = &operatorv1.NodeAddressAutodetection{
					FirstFound: &t,
				}
			}
			if v4pool.BlockSize == nil {
				var twentySix int32 = 26
				v4pool.BlockSize = &twentySix
			}
		}

		if v6pool != nil {
			if v6pool.Encapsulation == "" {
				v6pool.Encapsulation = operatorv1.EncapsulationNone
			}
			if v6pool.NATOutgoing == "" {
				v6pool.NATOutgoing = operatorv1.NATOutgoingDisabled
			}
			if v6pool.NodeSelector == "" {
				v6pool.NodeSelector = operatorv1.NodeSelectorDefault
			}
			if instance.Spec.CalicoNetwork.NodeAddressAutodetectionV6 == nil {
				// Default IPv6 address detection to "first found" if not specified.
				t := true
				instance.Spec.CalicoNetwork.NodeAddressAutodetectionV6 = &operatorv1.NodeAddressAutodetection{
					FirstFound: &t,
				}
			}
			if v6pool.BlockSize == nil {
				var oneTwentyTwo int32 = 122
				v6pool.BlockSize = &oneTwentyTwo
			}
		}

		if instance.Spec.CalicoNetwork.HostPorts == nil {
			hp := operatorv1.HostPortsEnabled
			instance.Spec.CalicoNetwork.HostPorts = &hp
		}

		if instance.Spec.CalicoNetwork.MultiInterfaceMode == nil {
			mm := operatorv1.MultiInterfaceModeNone
			instance.Spec.CalicoNetwork.MultiInterfaceMode = &mm
		}
	}

	// If not specified by the user, set the flex volume plugin location based on platform.
	if len(instance.Spec.FlexVolumePath) == 0 {
		if instance.Spec.KubernetesProvider == operatorv1.ProviderOpenShift {
			// In OpenShift 4.x, the location for flexvolume plugins has changed.
			// See: https://bugzilla.redhat.com/show_bug.cgi?id=1667606#c5
			instance.Spec.FlexVolumePath = "/etc/kubernetes/kubelet-plugins/volume/exec/"
		} else if instance.Spec.KubernetesProvider == operatorv1.ProviderGKE {
			instance.Spec.FlexVolumePath = "/home/kubernetes/flexvolume/"
		} else if instance.Spec.KubernetesProvider == operatorv1.ProviderAKS {
			instance.Spec.FlexVolumePath = "/etc/kubernetes/volumeplugins/"
		} else {
			instance.Spec.FlexVolumePath = "/usr/libexec/kubernetes/kubelet-plugins/volume/exec/"
		}
	}

	instance.Spec.NodeUpdateStrategy.Type = appsv1.RollingUpdateDaemonSetStrategyType

	var one = intstr.FromInt(1)

	if instance.Spec.NodeUpdateStrategy.RollingUpdate == nil {
		instance.Spec.NodeUpdateStrategy.RollingUpdate = &appsv1.RollingUpdateDaemonSet{
			MaxUnavailable: &one,
		}
	} else if instance.Spec.NodeUpdateStrategy.RollingUpdate.MaxUnavailable == nil {
		instance.Spec.NodeUpdateStrategy.RollingUpdate.MaxUnavailable = &one
	}

	return nil
}
