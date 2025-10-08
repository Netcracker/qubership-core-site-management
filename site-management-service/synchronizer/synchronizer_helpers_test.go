package synchronizer

import (
	"context"
	"testing"

	"github.com/netcracker/qubership-core-site-management/site-management-service/v2/domain"
	mdomain "github.com/netcracker/qubership-core-site-management/site-management-service/v2/paasMediationClient/domain"
)

func TestRouteIsGeneral(t *testing.T) {
	r := &mdomain.Route{Metadata: mdomain.Metadata{Annotations: map[string]string{"qubership.cloud/tenant.service.tenant.id": "GENERAL"}}}
	if !RouteIsGeneral(r) {
		t.Fatalf("expected general route")
	}
}

func TestHostBelongsToRoutes(t *testing.T) {
	routes := []mdomain.Route{{Spec: mdomain.RouteSpec{Host: "h.example.com"}}}
	if !hostBelongsToRoutes("h.example.com", &routes) {
		t.Fatalf("expected host to belong")
	}
}

func TestBuildSortedMapFromRoutes(t *testing.T) {
	routes := []mdomain.Route{
		{Metadata: mdomain.Metadata{Name: "r1"}, Spec: mdomain.RouteSpec{Service: mdomain.Target{Name: "svc"}}},
		{Metadata: mdomain.Metadata{Name: "r2"}, Spec: mdomain.RouteSpec{Service: mdomain.Target{Name: "svc"}}},
	}
	m := buildSortedMapFromRoutes(&routes)
	if len(m) != 1 {
		t.Fatalf("expected one key, got %d", len(m))
	}
}

func TestFilterRoutes_Simple(t *testing.T) {
	routes := []mdomain.Route{
		{Metadata: mdomain.Metadata{Name: "r1"}, Spec: mdomain.RouteSpec{Service: mdomain.Target{Name: "svc"}}},
	}
	filtered := filterRoutes(&routes)
	if len(*filtered) != 1 {
		t.Fatalf("expected 1, got %d", len(*filtered))
	}
}

func TestExtractSiteByHost(t *testing.T) {
	sites := domain.Sites{
		"default": {
			"svc": {domain.Address("https://a.example.com/")},
		},
		"alt": {
			"svc": {domain.Address("https://b.example.com/")},
		},
	}
	if got := extractSiteByHost(sites, "a.example.com"); got != "default" {
		t.Fatalf("expected default, got %s", got)
	}
	if got := extractSiteByHost(sites, "b.example.com"); got != "alt" {
		t.Fatalf("expected alt, got %s", got)
	}
	if got := extractSiteByHost(sites, "x.example.com"); got != "" {
		t.Fatalf("expected empty, got %s", got)
	}
}

func TestChangeServiceNameIfExists(t *testing.T) {
	tenant := &domain.TenantDns{Sites: domain.Sites{"s": {"from": {"h"}}}}
	if err := changeServiceNameIfExists("from", "to", "s", tenant); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := tenant.Sites["s"]["to"]; !ok {
		t.Fatalf("expected 'to' to exist")
	}
}

func TestAppendElemIfNotExists(t *testing.T) {
	elems := []string{"a"}
	appendElemIfNotExists(&elems, "a")
	appendElemIfNotExists(&elems, "b")
	if len(elems) != 2 {
		t.Fatalf("expected 2 elems, got %d", len(elems))
	}
}

func TestGetTenantRoutesFromOpenshiftRoutes(t *testing.T) {
	routes := []mdomain.Route{
		{Metadata: mdomain.Metadata{Name: "r1", Annotations: map[string]string{"qubership.cloud/tenant.service.tenant.id": "GENERAL"}}, Spec: mdomain.RouteSpec{Host: "c.example.com"}},
		{Metadata: mdomain.Metadata{Name: "r2", Annotations: map[string]string{"qubership.cloud/tenant.service.tenant.id": "obj-1"}}, Spec: mdomain.RouteSpec{Host: "t1.example.com"}},
		{Metadata: mdomain.Metadata{Name: "r3", Annotations: map[string]string{"qubership.cloud/tenant.service.tenant.id": "obj-1"}}, Spec: mdomain.RouteSpec{Host: "t2.example.com"}},
	}
	common, tenants := getTenantRoutesFromOpenshiftRoutes(context.Background(), &routes)
	if len(common) != 1 {
		t.Fatalf("expected 1 common, got %d", len(common))
	}
	if hosts, ok := tenants["obj-1"]; !ok || len(hosts) != 2 {
		t.Fatalf("expected tenant obj-1 to have 2 hosts")
	}
}
