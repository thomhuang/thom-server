package data

import "testing"

func TestErrNoRecordMessage(t *testing.T) {
	if ErrNoRecord.Error() != "data: no matching record found" {
		t.Fatalf("unexpected ErrNoRecord message: %q", ErrNoRecord.Error())
	}
}
