package clock

import "time"

type Real struct{}

func (Real) Tick(d time.Duration) (ticks <-chan time.Time, stop func()) {
	t := time.NewTicker(d)
	return t.C, t.Stop
}
