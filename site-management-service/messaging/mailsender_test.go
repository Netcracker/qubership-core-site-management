package messaging

import (
	"context"
	"testing"

	"github.com/netcracker/qubership-core-site-management/site-management-service/v2/domain"
	mdomain "github.com/netcracker/qubership-core-site-management/site-management-service/v2/paasMediationClient/domain"
)

func TestGenerateTextForTenantUpdate(t *testing.T) {
	// minimal sender
	sender := &MailSender{userEmail: "from@example.com", messageContent: "%s %s %s %s"}

	tenant := domain.TenantDns{
		TenantId:    "t1",
		TenantAdmin: "to@example.com",
		Sites: domain.Sites{
			"default": {
				"svc1": {"a.example.com"},
			},
		},
	}
	common := []mdomain.Route{{
		Spec: mdomain.RouteSpec{Host: "c.example.com", Service: mdomain.Target{Name: "svc2"}},
	}}
	txt := sender.GenerateTextForTenantUpdate(context.Background(), tenant, common)
	if txt == "" {
		t.Fatalf("expected non-empty message")
	}
}
