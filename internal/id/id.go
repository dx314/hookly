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

// RequestIDLength is the length of proxied-request IDs (matched on the stream only).
const RequestIDLength = 21

// NewRequestID generates an ID for one proxied HTTP request.
func NewRequestID() string {
	id, _ := gonanoid.New(RequestIDLength)
	return id
}
