package synchronizer

import (
	"testing"

	"github.com/netcracker/qubership-core-site-management/site-management-service/v2/domain"
	"github.com/stretchr/testify/assert"
)

const baselineDomain = "autotest-1-spec-core-ci-4.test.example"

func TestSitesWithoutBaselineHosts_DropsOnlyHostsOfTheBaselineDomain(t *testing.T) {
	stored := &domain.TenantDns{Sites: domain.Sites{
		"default": domain.Services{
			"public-gateway":      domain.AddressList{"https://public-gateway." + baselineDomain + "/"},
			"tenant-self-service": domain.AddressList{"tenant-self-service." + baselineDomain},
			"satellite-app":       domain.AddressList{"https://app.satellite.example/"},
		},
	}}

	sites, dropped := sitesWithoutBaselineHosts(stored, baselineDomain)

	assert.Equal(t, 2, dropped)
	assert.Equal(t, domain.Sites{"default": domain.Services{
		"satellite-app": domain.AddressList{"https://app.satellite.example/"},
	}}, sites)
}

func TestSitesWithoutBaselineHosts_KeepsOtherAddressesOfAServiceThatHasSeveral(t *testing.T) {
	stored := &domain.TenantDns{Sites: domain.Sites{"default": domain.Services{
		"public-gateway": domain.AddressList{"public-gateway." + baselineDomain, "gateway.satellite.example"},
	}}}

	sites, dropped := sitesWithoutBaselineHosts(stored, baselineDomain)

	assert.Equal(t, 1, dropped)
	assert.Equal(t, domain.AddressList{"gateway.satellite.example"}, sites["default"]["public-gateway"])
}

func TestSitesWithoutBaselineHosts_RemovesAServiceAndASiteLeftEmpty(t *testing.T) {
	stored := &domain.TenantDns{Sites: domain.Sites{
		"default": domain.Services{"public-gateway": domain.AddressList{"public-gateway." + baselineDomain}},
		"second":  domain.Services{"app": domain.AddressList{"app.satellite.example"}},
	}}

	sites, dropped := sitesWithoutBaselineHosts(stored, baselineDomain)

	assert.Equal(t, 1, dropped)
	assert.NotContains(t, sites, "default", "a site without services must not stay, it would be regenerated on a domain change")
	assert.Equal(t, domain.AddressList{"app.satellite.example"}, sites["second"]["app"])
}

func TestSitesWithoutBaselineHosts_IgnoresCaseAndPort(t *testing.T) {
	stored := &domain.TenantDns{Sites: domain.Sites{"default": domain.Services{
		"upper": domain.AddressList{"https://PUBLIC-GATEWAY." + baselineDomain + "/"},
		"port":  domain.AddressList{"https://public-gateway." + baselineDomain + ":8443/"},
	}}}

	sites, dropped := sitesWithoutBaselineHosts(stored, "AUTOTEST-1-spec-core-ci-4.test.example")

	assert.Equal(t, 2, dropped)
	assert.Empty(t, sites)
}

func TestSitesWithoutBaselineHosts_DoesNotMatchALookalikeDomain(t *testing.T) {
	lookalike := "public-gateway.sat-" + baselineDomain
	stored := &domain.TenantDns{Sites: domain.Sites{"default": domain.Services{
		"public-gateway": domain.AddressList{domain.Address(lookalike)},
	}}}

	sites, dropped := sitesWithoutBaselineHosts(stored, baselineDomain)

	assert.Equal(t, 0, dropped)
	assert.Equal(t, domain.AddressList{domain.Address(lookalike)}, sites["default"]["public-gateway"])
}

func TestSitesWithoutBaselineHosts_EmptyDomainDropsNothing(t *testing.T) {
	stored := &domain.TenantDns{Sites: domain.Sites{"default": domain.Services{
		"public-gateway": domain.AddressList{"public-gateway." + baselineDomain},
	}}}

	sites, dropped := sitesWithoutBaselineHosts(stored, "")

	assert.Equal(t, 0, dropped)
	assert.Equal(t, stored.Sites, sites)
}

func TestSitesWithoutBaselineHosts_NoStoredTenantGivesEmptySites(t *testing.T) {
	sites, dropped := sitesWithoutBaselineHosts(nil, baselineDomain)

	assert.Equal(t, 0, dropped)
	assert.NotNil(t, sites)
	assert.Empty(t, sites)
}

func TestSitesWithoutBaselineHosts_DoesNotChangeTheStoredTenant(t *testing.T) {
	stored := &domain.TenantDns{Sites: domain.Sites{"default": domain.Services{
		"public-gateway": domain.AddressList{"public-gateway." + baselineDomain, "gateway.satellite.example"},
	}}}

	_, _ = sitesWithoutBaselineHosts(stored, baselineDomain)

	assert.Equal(t, domain.AddressList{"public-gateway." + baselineDomain, "gateway.satellite.example"},
		stored.Sites["default"]["public-gateway"])
}
