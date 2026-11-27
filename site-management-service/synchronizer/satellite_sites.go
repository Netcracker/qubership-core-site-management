package synchronizer

import (
	"context"
	"net"
	"strings"

	"github.com/netcracker/qubership-core-site-management/site-management-service/v2/domain"
	"github.com/netcracker/qubership-core-site-management/site-management-service/v2/tm"
)

// satelliteSites returns the sites a composite satellite stores for an active tenant that it learned from a
// tenant-manager watch event.
//
// The event carries the domain of the baseline. A satellite must not turn that domain into tenant hosts: the baseline
// builds the same hosts, the cluster accepts a host once, and whichever cell creates it first owns it. So the satellite
// keeps the routes it already stores, without the hosts of the baseline domain, and generates no default routes.
//
// This is the interim guard of the composite route ownership design. It is replaced by explicit route assignment per
// cell, where a satellite creates a tenant host only when it is assigned one.
func (s *Synchronizer) satelliteSites(ctx context.Context, tenant tm.Tenant) domain.Sites {
	stored, err := s.dao.FindByTenantId(ctx, tenant.ObjectId)
	if err != nil {
		logger.InfoC(ctx, "Tenant %s has no routes stored on this satellite yet: %v", tenant.ObjectId, err)
		stored = nil
	}
	sites, dropped := sitesWithoutBaselineHosts(stored, tenant.DomainName)
	if dropped > 0 {
		logger.InfoC(ctx, "Satellite drops %d host(s) of the baseline domain %s from the routes of tenant %s: "+
			"tenant hosts of the baseline domain are created by the baseline only", dropped, tenant.DomainName, tenant.ObjectId)
	}
	return sites
}

// sitesWithoutBaselineHosts returns a copy of the stored sites without the addresses that belong to the baseline
// domain, and the number of addresses it dropped. A service left without addresses is removed, and so is a site left
// without services. The stored value is not changed. A nil stored tenant gives empty sites.
func sitesWithoutBaselineHosts(stored *domain.TenantDns, baselineDomain string) (domain.Sites, int) {
	result := domain.Sites{}
	if stored == nil {
		return result, 0
	}
	dropped := 0
	for site, services := range stored.Sites {
		keptServices := domain.Services{}
		for service, addresses := range services {
			kept := make(domain.AddressList, 0, len(addresses))
			for _, address := range addresses {
				if isHostOfDomain(address, baselineDomain) {
					dropped++
					continue
				}
				kept = append(kept, address)
			}
			if len(kept) > 0 {
				keptServices[service] = kept
			}
		}
		if len(keptServices) > 0 {
			result[site] = keptServices
		}
	}
	return result, dropped
}

// isHostOfDomain reports whether the host of the address is the domain or a host below it. The comparison ignores
// case and port. An empty domain matches nothing.
func isHostOfDomain(address domain.Address, baselineDomain string) bool {
	suffix := strings.ToLower(strings.Trim(baselineDomain, ". "))
	if suffix == "" {
		return false
	}
	host := strings.ToLower(address.Host())
	if withoutPort, _, err := net.SplitHostPort(host); err == nil {
		host = withoutPort
	}
	return host == suffix || strings.HasSuffix(host, "."+suffix)
}
