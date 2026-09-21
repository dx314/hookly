package cli

import (
	"strings"
	"testing"
	"time"

	"hooks.dx314.com/internal/logincode"
)

func TestReadLoginCode(t *testing.T) {
	const state = "this-login"
	good, _ := logincode.Encode(logincode.Payload{Token: "hk_token", UserID: "42", Username: "alex", State: state})
	otherAttempt, _ := logincode.Encode(logincode.Payload{Token: "hk_other", UserID: "1", Username: "mallory", State: "another-login"})

	// Garbage and a code from a different login attempt are skipped; the first
	// valid code for this attempt wins, even with whitespace around it.
	input := strings.NewReader("\nnot a code\n" + otherAttempt + "\n  " + good + "  \n")
	resultCh := make(chan *LoginResult, 1)
	go readLoginCode(input, state, resultCh)

	select {
	case got := <-resultCh:
		if got.Token != "hk_token" || got.UserID != "42" || got.Username != "alex" {
			t.Errorf("got %+v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no login result from pasted code")
	}
}

func TestReadLoginCodeIgnoresWrongState(t *testing.T) {
	otherAttempt, _ := logincode.Encode(logincode.Payload{Token: "hk_other", State: "another-login"})
	resultCh := make(chan *LoginResult, 1)
	readLoginCode(strings.NewReader(otherAttempt+"\n"), "this-login", resultCh)

	select {
	case got := <-resultCh:
		t.Fatalf("accepted a code from another login attempt: %+v", got)
	default:
	}
}
