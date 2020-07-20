package kubeadm

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"strings"

	operatorv1 "github.com/tigera/operator/pkg/apis/operator/v1"
	"github.com/tigera/operator/pkg/controller/migration/utils"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// KubeadmConfigConfigMap is defined in k8s.io/kubernetes, which we can't import due to versioning issues.
	kubeadmConfigMap = "kubeadm-config"
)

func Convert(ctx context.Context, client client.Client, i *operatorv1.Installation) error {
	kubeadmConfig := &v1.ConfigMap{}
	if err := client.Get(ctx, types.NamespacedName{
		Name:      kubeadmConfigMap,
		Namespace: metav1.NamespaceSystem,
	}, kubeadmConfig); err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("Unable to read kubeadm config map: %s", err.Error())
		}
		kubeadmConfig = nil
	}

	platformCIDRs, err := extractKubeadmCIDRs(kubeadmConfig)
	if err != nil {
		return err
	}
	return utils.MergePlatformPodCIDRs(i, platformCIDRs)
}

// extractKubeadmCIDRs looks through the config map and parses lines starting with 'podSubnet'.
func extractKubeadmCIDRs(kubeadmConfig *v1.ConfigMap) ([]string, error) {
	var line []string
	var foundCIDRs []string

	// Look through the config map for a line starting with 'podSubnet', then assign the right variable
	// according to the IP family of the matching string.
	re := regexp.MustCompile(`podSubnet: (.*)`)
	for _, l := range kubeadmConfig.Data {
		if line = re.FindStringSubmatch(l); line != nil {
			break
		}
	}

	if len(line) != 0 {
		// IPv4 and IPv6 CIDRs will be separated by a comma in a dual stack setup.
		for _, cidr := range strings.Split(line[1], ",") {
			_, _, err := net.ParseCIDR(cidr)
			if err != nil {
				return nil, err
			}

			// Parsed successfully. Add it to the list.
			foundCIDRs = append(foundCIDRs, cidr)
		}
	}

	return foundCIDRs, nil
}
