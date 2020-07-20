package openshift

import (
	"context"
	"fmt"

	operatorv1 "github.com/tigera/operator/pkg/apis/operator/v1"
	"github.com/tigera/operator/pkg/controller/migration/utils"

	configv1 "github.com/openshift/api/config/v1"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var openshiftNetworkConfig = "cluster"

func Convert(ctx context.Context, client client.Client, i *operatorv1.Installation) error {
	var openshiftConfig = &configv1.Network{}
	// If configured to run in openshift, then also fetch the openshift configuration API.
	err := client.Get(ctx, types.NamespacedName{Name: openshiftNetworkConfig}, openshiftConfig)
	if err != nil {
		return fmt.Errorf("Unable to read openshift network configuration: %s", err.Error())
	}

	if i.Spec.CalicoNetwork == nil {
		i.Spec.CalicoNetwork = &operatorv1.CalicoNetworkSpec{}
	}

	platformCIDRs := []string{}
	for _, c := range openshiftConfig.Spec.ClusterNetwork {
		platformCIDRs = append(platformCIDRs, c.CIDR)
	}
	return utils.MergePlatformPodCIDRs(i, platformCIDRs)
}
