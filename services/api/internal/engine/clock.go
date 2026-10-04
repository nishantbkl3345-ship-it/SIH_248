package engine

import (
	"errors"
	"time"
)

// Speed limits, in thousandths: 250 is quarter speed, 20000 is 20x.
const (
	MinSpeedMilli = 250
	MaxSpeedMilli = 20_000
)

var ErrSpeedOutOfRange = errors.New("engine: speed out of range")

// Clock maps wall time to simulation time. It holds no reference to the
// system clock: every method is told what the wall time is.
//
// Speed is an integer number of thousandths so the mapping is exact integer
// arithmetic, identical on every machine.
type Clock struct {
	running    bool
	speedMilli int64
	baseSim    SimMs     // simulation time at baseWall
	baseWall   time.Time // wall time of the last start, resume or speed change
}

func NewClock(speedMilli int64) Clock {
	return Clock{speedMilli: speedMilli}
}

func (c *Clock) Running() bool     { return c.running }
func (c *Clock) SpeedMilli() int64 { return c.speedMilli }

// Now returns the simulation time at the given wall time. A wall time earlier
// than the last anchor never moves simulation time backwards.
func (c *Clock) Now(wall time.Time) SimMs {
	if !c.running {
		return c.baseSim
	}
	elapsed := wall.Sub(c.baseWall).Milliseconds()
	if elapsed < 0 {
		elapsed = 0
	}
	return c.baseSim + elapsed*c.speedMilli/1000
}

// WallAt is the inverse of Now: the earliest wall time at which simulation
// time reaches sim, under the current anchor and speed. Scheduled events are
// stamped with it, so their wall time is the moment they were due rather than
// the moment someone happened to ask.
func (c *Clock) WallAt(sim SimMs) time.Time {
	if !c.running || sim <= c.baseSim {
		return c.baseWall
	}
	// Round up, so that Now(WallAt(sim)) >= sim.
	elapsedMs := ((sim-c.baseSim)*1000 + c.speedMilli - 1) / c.speedMilli
	return c.baseWall.Add(time.Duration(elapsedMs) * time.Millisecond)
}

// Resume starts (or restarts) the clock from where it stopped.
func (c *Clock) Resume(wall time.Time) {
	if c.running {
		return
	}
	c.baseWall = wall
	c.running = true
}

// Pause freezes simulation time at its value for the given wall time.
func (c *Clock) Pause(wall time.Time) {
	c.baseSim = c.Now(wall)
	c.running = false
}

// SetSpeed changes the rate from the given wall time onward; simulation time
// already elapsed is unaffected.
func (c *Clock) SetSpeed(wall time.Time, speedMilli int64) error {
	if speedMilli < MinSpeedMilli || speedMilli > MaxSpeedMilli {
		return ErrSpeedOutOfRange
	}
	c.baseSim = c.Now(wall)
	c.baseWall = wall
	c.speedMilli = speedMilli
	return nil
}

// StopAt freezes the clock at an exact simulation time.
func (c *Clock) StopAt(sim SimMs) {
	c.baseSim = sim
	c.running = false
}
