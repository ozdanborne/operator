package utils

import (
	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
)

var _ = Describe("Testing core-controller installation", func() {
	table.DescribeTable("test cidrWithinCidr function",
		func(CIDR, pool string, expectedResult bool) {
			if expectedResult {
				Expect(cidrWithinCidr(CIDR, pool)).To(BeTrue(), "Expected pool %s to be within CIDR %s", pool, CIDR)
			} else {
				Expect(cidrWithinCidr(CIDR, pool)).To(BeFalse(), "Expected pool %s to not be within CIDR %s", pool, CIDR)
			}
		},

		table.Entry("Default as CIDR and pool", "192.168.0.0/16", "192.168.0.0/16", true),
		table.Entry("Pool larger than CIDR should fail", "192.168.0.0/16", "192.168.0.0/15", false),
		table.Entry("Pool larger than CIDR should fail", "192.168.2.0/24", "192.168.0.0/16", false),
		table.Entry("Non overlapping CIDR and pool should fail", "192.168.0.0/16", "172.168.0.0/16", false),
		table.Entry("CIDR with smaller pool", "192.168.0.0/16", "192.168.2.0/24", true),
		table.Entry("IPv6 matching CIDR and pool", "fd00:1234::/32", "fd00:1234::/32", true),
		table.Entry("IPv6 Pool larger than CIDR should fail", "fd00:1234::/32", "fd00:1234::/31", false),
		table.Entry("IPv6 Pool larger than CIDR should fail", "fd00:1234:5600::/40", "fd00:1234::/32", false),
		table.Entry("IPv6 Non overlapping CIDR and pool should fail", "fd00:1234::/32", "fd00:5678::/32", false),
		table.Entry("IPv6 CIDR with smaller pool", "fd00:1234::/32", "fd00:1234:5600::/40", true),
	)
})
