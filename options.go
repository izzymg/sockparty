package sockparty

import (
	"time"

	"golang.org/x/time/rate"
)

// DefaultOptions generates party options with defaults. Use if you're just testing.
func DefaultOptions() *Options {
	return &Options{
		AllowCrossOrigin: false,
		RateLimiter:      rate.NewLimiter(rate.Every(time.Millisecond*100), 5),
		PingFrequency:    time.Second * 15,
		PingTimeout:      time.Second * 10,
	}
}

// Options configures a party's settings.
type Options struct {

	// Allow cross origin socket requests
	AllowCrossOrigin bool

	// Limiter used against incoming client messages.
	RateLimiter *rate.Limiter

	// Determines how frequently users are pinged. Set to zero for no pings.
	PingFrequency time.Duration
	// Determines how long to wait on a ping before assuming the connection is dead.
	PingTimeout time.Duration
}
