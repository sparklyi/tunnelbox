// Package humancheck wires the bouncer drop-in guard into tunnelbox.
//
// The key lives next to the database so that a restart does not invalidate the
// passes that are still in flight, and so that a self-hosted install needs no
// environment variable. Nothing else is configured: the guard serves its own
// endpoints and browser widget.
package humancheck

import (
	"github.com/sparklyi/bouncer"
	"github.com/sparklyi/bouncer/httpx"
)

// Check owns the gate lifecycle behind the guard.
type Check struct {
	Guard *httpx.Guard
	gate  *bouncer.Gate
}

// New builds the guard, creating the signing key at path on first run.
func New(path string, secure bool) (*Check, error) {
	gate, err := bouncer.New(bouncer.WithSecretFile(path))
	if err != nil {
		return nil, err
	}
	return &Check{
		// Every login attempt must pay for a fresh challenge: a pass that stays
		// valid for ten minutes would let one solve amortise thousands of
		// password guesses.
		Guard: httpx.NewGuard(gate, httpx.WithSecure(secure), httpx.WithPassSingleUse()),
		gate:  gate,
	}, nil
}

// Close releases the gate and its default store.
func (c *Check) Close() error {
	if c == nil || c.gate == nil {
		return nil
	}
	return c.gate.Close()
}
