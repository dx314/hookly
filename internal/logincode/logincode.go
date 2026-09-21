// Package logincode encodes the result of a CLI login as a single string that
// can be pasted into a terminal. It is what `hookly login` accepts when the
// browser can't reach the CLI's local callback (e.g. the CLI runs on a server
// over SSH and the browser is on another machine).
package logincode

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
)

// Prefix marks a string as a hookly login code.
const Prefix = "hkc_"

// ErrInvalid is returned for strings that are not a login code.
var ErrInvalid = errors.New("not a valid login code")

// Payload is what a login code carries: the same values the local callback receives.
type Payload struct {
	Token    string `json:"token"`
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	State    string `json:"state"`
}

// Encode returns the login code for a payload.
func Encode(p Payload) (string, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	return Prefix + base64.RawURLEncoding.EncodeToString(data), nil
}

// Decode parses a pasted login code. Surrounding whitespace and line breaks
// added by terminals or copy/paste are ignored.
func Decode(code string) (Payload, error) {
	code = strings.Join(strings.Fields(code), "")
	if !strings.HasPrefix(code, Prefix) {
		return Payload{}, ErrInvalid
	}
	data, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(code, Prefix))
	if err != nil {
		return Payload{}, ErrInvalid
	}
	var p Payload
	if err := json.Unmarshal(data, &p); err != nil {
		return Payload{}, ErrInvalid
	}
	if p.Token == "" || p.State == "" {
		return Payload{}, ErrInvalid
	}
	return p, nil
}
