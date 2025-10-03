package synchronizer

import (
	"testing"

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
