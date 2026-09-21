package logincode

import (
	"errors"
	"strings"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	want := Payload{Token: "hk_abc|123", UserID: "42", Username: "alex", State: "CVrR48E7BEfvUQ2cqbqNZA=="}
	code, err := Encode(want)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !strings.HasPrefix(code, Prefix) || strings.ContainsAny(code, "+/= \n") {
		t.Errorf("code is not terminal/URL safe: %q", code)
	}

	// Pasting often adds whitespace or wraps the line
	pasted := "  " + code[:10] + "\n" + code[10:] + " \r\n"
	got, err := Decode(pasted)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestDecodeRejectsGarbage(t *testing.T) {
	noToken, _ := Encode(Payload{State: "s"})
	for _, in := range []string{"", "hello", "hk_sometoken", Prefix, Prefix + "!!!", Prefix + "e30", noToken} {
		if _, err := Decode(in); !errors.Is(err, ErrInvalid) {
			t.Errorf("Decode(%q) = %v, want ErrInvalid", in, err)
		}
	}
}
