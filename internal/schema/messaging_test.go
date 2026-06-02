package schema

import "testing"

func TestNormalize_EmptyString(t *testing.T) {
	if got := Normalize(""); got != Messaging {
		t.Fatalf("expected %q, got %q", Messaging, got)
	}
}

func TestNormalize_Whitespace(t *testing.T) {
	if got := Normalize("   "); got != Messaging {
		t.Fatalf("expected %q, got %q", Messaging, got)
	}
}

func TestNormalize_ExplicitMessaging(t *testing.T) {
	if got := Normalize(Messaging); got != Messaging {
		t.Fatalf("expected %q, got %q", Messaging, got)
	}
}

func TestNormalize_OtherTypeUnchanged(t *testing.T) {
	for _, st := range []string{"iso20022", "hl7fhir", "edifact", "custom:gov-rs"} {
		if got := Normalize(st); got != st {
			t.Fatalf("Normalize(%q): expected unchanged, got %q", st, got)
		}
	}
}
