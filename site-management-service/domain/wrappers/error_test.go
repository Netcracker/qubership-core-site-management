package errors

import "testing"

func TestErrorWrapper_Error(t *testing.T) {
	e := ErrorWrapper{StatusCode: 400, Message: "oops"}
	if e.Error() != "oops" {
		t.Fatalf("unexpected error string")
	}
}
