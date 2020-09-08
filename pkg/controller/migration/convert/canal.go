package convert

import (
	"encoding/json"
	"fmt"

	operatorv1 "github.com/tigera/operator/pkg/apis/operator/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func handleCanal(c *components, install *Installation) error {
	v := getVolume(c.node.Spec.Template.Spec, "flannel-cfg")
	if v == nil {
		return nil
	}

	// TODO: validate the configmap is actually used?
	// load the config
	if v.ConfigMap == nil {
		return ErrIncompatibleCluster{"canal must load config via configmap"}
	}
	cm := corev1.ConfigMap{}
	if err := c.client.Get(ctx, types.NamespacedName{
		Namespace: "kube-system",
		Name:      v.ConfigMap.Name,
	}, &cm); err != nil {
		return err
	}

	bits := []byte(cm.Data["net-conf.json"])
	fc := flannelConfig{}
	if err := json.Unmarshal(bits, &fc); err != nil {
		return fmt.Errorf("failed to parse '%s': %v", cm.Data["net-conf.json"], err)
	}

	if t, ok := fc.Backend["Type"]; ok && t != "vxlan" {
		return ErrIncompatibleCluster{"only backend vxlan supported"}
	}

	install.Spec.CalicoNetwork.IPPools = []operatorv1.IPPool{{
		CIDR:          fc.Network,
		Encapsulation: operatorv1.EncapsulationVXLAN,
	}}

	return nil
}

type flannelConfig struct {
	Network string
	Backend map[string]string
}
