// Package resources contains the pure capacity arithmetic shared by the
// orchestration phases.
package resources

import (
	"fmt"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

// Capacity describes one named resource on a host or project. Effective is
// the value to use for admission; Configured and Observed retain the two
// inputs from which a caller may have derived it.
type Capacity struct {
	Name       string
	Configured int
	Observed   int
	Effective  int
	Reserved   int
	Unknown    bool
}

// Reservation is a requested or existing reservation for a named resource.
// State is retained for the caller's durable record; Reserve treats every
// supplied reservation as consuming capacity.
type Reservation struct {
	Run   core.RunID
	Team  core.TeamID
	Name  string
	Count int
	State string
}

// Reserve validates that reservations fit within the supplied effective
// capacities. It is deliberately pure: neither argument is modified, and no
// host or process state is inspected.
func Reserve(capacities []Capacity, reservations []Reservation) error {
	available := make(map[string]int, len(capacities))
	for _, capacity := range capacities {
		if capacity.Name == "" {
			return capacityError("capacity name is empty")
		}
		if _, exists := available[capacity.Name]; exists {
			return capacityError("capacity %q is duplicated", capacity.Name)
		}
		if capacity.Configured < 0 || capacity.Observed < 0 || capacity.Effective < 1 || capacity.Reserved < 0 {
			return capacityError("capacity %q has invalid limits", capacity.Name)
		}
		if capacity.Unknown {
			return capacityError("capacity %q is unknown", capacity.Name)
		}
		if capacity.Reserved > capacity.Effective {
			return capacityError("capacity %q is already over-reserved", capacity.Name)
		}
		available[capacity.Name] = capacity.Effective - capacity.Reserved
	}

	for _, reservation := range reservations {
		if reservation.Name == "" {
			return capacityError("reservation name is empty")
		}
		if reservation.Count < 0 {
			return capacityError("reservation %q has a negative count", reservation.Name)
		}
		remaining, exists := available[reservation.Name]
		if !exists {
			return capacityError("reservation %q has no capacity", reservation.Name)
		}
		if reservation.Count > remaining {
			return capacityError("reservation %q exceeds available capacity", reservation.Name)
		}
		available[reservation.Name] = remaining - reservation.Count
	}

	return nil
}

func capacityError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", core.ErrCapacity, fmt.Sprintf(format, args...))
}
