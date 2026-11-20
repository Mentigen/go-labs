package positions

import "time"

type RealClock struct{}

func (RealClock) Now() time.Time                          { return time.Now() }
func (RealClock) NewTicker(interval time.Duration) Ticker { return ticker{time.NewTicker(interval)} }

type ticker struct{ *time.Ticker }

func (t ticker) C() <-chan time.Time { return t.Ticker.C }
