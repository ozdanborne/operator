package utils

import (
	"fmt"
	"log"
	"net"

	operatorv1 "github.com/tigera/operator/pkg/apis/operator/v1"
)

func MergePlatformPodCIDRs(i *operatorv1.Installation, platformCIDRs []string) error {
	// If IPPools is nil, add IPPool with CIDRs detected from platform configuration.
	if i.Spec.CalicoNetwork.IPPools == nil {
		if len(platformCIDRs) == 0 {
			// If the platform has no CIDRs defined as well, then return and let the
			// normal defaulting happen.
			return nil
		}
		v4found := false
		v6found := false

		// Currently we only support a single IPv4 and a single IPv6 CIDR configured via the operator.
		// So, grab the 1st IPv4 and IPv6 cidrs we find and use those. This will allow the
		// operator to work in situations where there are more than one of each.
		for _, c := range platformCIDRs {
			addr, _, err := net.ParseCIDR(c)
			if err != nil {
				log.Print(err, "Failed to parse platform's pod network CIDR.")
				continue
			}

			if addr.To4() == nil {
				if v6found {
					continue
				}
				v6found = true
				i.Spec.CalicoNetwork.IPPools = append(i.Spec.CalicoNetwork.IPPools,
					operatorv1.IPPool{CIDR: c})
			} else {
				if v4found {
					continue
				}
				v4found = true
				i.Spec.CalicoNetwork.IPPools = append(i.Spec.CalicoNetwork.IPPools,
					operatorv1.IPPool{CIDR: c})
			}
			if v6found && v4found {
				break
			}
		}
	} else if len(i.Spec.CalicoNetwork.IPPools) == 0 {
		// Empty IPPools list so nothing to do.
		return nil
	} else {
		// Pools are configured on the Installation. Make sure they are compatible with
		// the configuration set in the underlying Kubernetes platform.
		for _, pool := range i.Spec.CalicoNetwork.IPPools {
			within := false
			for _, c := range platformCIDRs {
				within = within || cidrWithinCidr(c, pool.CIDR)
			}
			if !within {
				return fmt.Errorf("IPPool %v is not within the platform's configured pod network CIDR(s)", pool.CIDR)
			}
		}
	}
	return nil
}

// cidrWithinCidr checks that all IPs in the pool passed in are within the
// passed in CIDR
func cidrWithinCidr(cidr, pool string) bool {
	_, cNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}
	_, pNet, err := net.ParseCIDR(pool)
	if err != nil {
		return false
	}
	ipMin := pNet.IP
	pOnes, _ := pNet.Mask.Size()
	cOnes, _ := cNet.Mask.Size()

	// If the cidr contains the network (1st) address of the pool and the
	// prefix on the pool is larger than or equal to the cidr prefix (the pool size is
	// smaller than the cidr) then the pool network is within the cidr network.
	if cNet.Contains(ipMin) && pOnes >= cOnes {
		return true
	}
	return false
}
