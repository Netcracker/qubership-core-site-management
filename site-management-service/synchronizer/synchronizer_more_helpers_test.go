package synchronizer

import (
	"testing"

	mdomain "github.com/netcracker/qubership-core-site-management/site-management-service/v2/paasMediationClient/domain"
)

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
