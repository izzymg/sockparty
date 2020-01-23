package sockparty

import (
	"time"

	"golang.org/x/time/rate"
)

// DefaultOptions generates party options with defaults. Use if you're just testing.
func DefaultOptions() *Options {
	return &Options{
		RateLimiter:   rate.NewLimiter(rate.Every(time.Millisecond*100), 5),
		DoPing:        true,
		PingFrequency: time.Second * 15,
		PingTimeout:   time.Second * 10,
	}
}

// Options configures a party's settings.
type Options struct {
	// Limiter used against incoming client messages.
	RateLimiter *rate.Limiter

	// Determines whether clients should be pinged.
	DoPing bool

	// Determines how frequently users are pinged.
	PingFrequency time.Duration
	// Determines how long to wait on a ping before assuming the connection is dead.
	PingTimeout time.Duration
}
