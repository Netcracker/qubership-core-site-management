package exceptions

import "testing"

func TestTenantNotFoundError(t *testing.T) {
	err := NewTenantNotFoundError("t1")
	if err.Error() == "" {
		t.Fatalf("expected message")
	}
}

func TestTenantIsNotActiveError(t *testing.T) {
	err := NewTenantIsNotActiveError("t1")
	if err.Error() == "" {
		t.Fatalf("expected message")
	}
}
