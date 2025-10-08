package domain

import (
	"testing"

	mdomain "github.com/netcracker/qubership-core-site-management/site-management-service/v2/paasMediationClient/domain"
)

func TestAddressHostAndPath(t *testing.T) {
	a := Address("example.com/foo")
	if a.Host() != "example.com" {
		t.Fatalf("host expected example.com, got %s", a.Host())
	}
	if Address("example.com/bar").Path() != "/bar" {
		t.Fatalf("path expected /bar")
	}
}

func TestFlattenAddressesToHosts(t *testing.T) {
	tnt := &TenantDns{Sites: Sites{"default": {"svc": {Address("https://h/p")}}}}
	tnt.FlattenAddressesToHosts()
	if tnt.Sites["default"]["svc"][0] != "h" {
		t.Fatalf("expected host only")
	}
}

func TestMergeDatabaseSchemeWithGeneralRoutes(t *testing.T) {
	db := &TenantDns{Sites: Sites{"default": {"svc": {Address("dbhost")}}}}
	routes := &[]mdomain.Route{{Spec: mdomain.RouteSpec{Host: "common"}, Metadata: mdomain.Metadata{}}}
	merged := MergeDatabaseSchemeWithGeneralRoutes(db, routes)
	if len(merged.Sites["default"]["svc"]) == 0 {
		t.Fatalf("expected db service to persist")
	}
}

func TestFilterBySite(t *testing.T) {
	tnt := &TenantDns{Sites: Sites{"default": {}, "other": {}}}
	out := FilterBySite(tnt, "default")
	if len(out.Sites) != 1 || out.Sites["default"] == nil {
		t.Fatalf("expected only default site")
	}
}

func TestFromRoutesAndAppendToRoutes(t *testing.T) {
	routes := []mdomain.Route{
		{Metadata: mdomain.Metadata{Annotations: map[string]string{"qubership.cloud/tenant.service.tenant.id": "TEN"}}, Spec: mdomain.RouteSpec{Host: "h1", Service: mdomain.Target{Name: "svc"}}},
	}
	tenants := FromRoutes(&routes)
	if len(*tenants) != 1 {
		t.Fatalf("expected one tenant")
	}
	// Append back to routes
	out := (*tenants)[0].AppendToRoutes(&[]mdomain.Route{})
	if len(*out) == 0 {
		t.Fatalf("expected at least one route appended")
	}
}

func TestSortTenantDns(t *testing.T) {
	list := []TenantDns{{TenantId: "b"}, {TenantId: "a"}}
	SortTenantDns(list)
	if list[0].TenantId != "a" {
		t.Fatalf("expected sorted by tenant id")
	}
}
