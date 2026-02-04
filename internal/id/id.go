// Package id provides centralized ID generation for hookly.
package id

import gonanoid "github.com/matoous/go-nanoid/v2"

// EndpointIDLength is the length of endpoint IDs.
// 64 characters with 64-char alphabet provides ~384 bits of entropy.
const EndpointIDLength = 64

// NewEndpointID generates a new endpoint ID with maximum security.
func NewEndpointID() string {
	id, _ := gonanoid.New(EndpointIDLength)
	return id
}
