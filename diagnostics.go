package rhttp

// OnInvalidConfig is called when a constructor receives configuration it cannot
// apply and falls back to a pass-through. It reports the component and the
// reason, so a protection that is absent becomes visible at startup rather than
// during the incident it was meant to prevent.
//
// Nil by default, which keeps the fallback silent and the behavior of earlier
// versions unchanged. Assign it once during startup, before constructing any
// client or limiter: it is a plain package variable, and mutating it while
// another goroutine builds middleware is a data race.
//
//	rhttp.OnInvalidConfig = func(component, reason string) {
//		log.Printf("rhttp: %s is inert: %s", component, reason)
//	}
//
// Only components whose absence loses a protection report here. A nil
// MetricsConfig.Recorder or LoggingConfig.Logger means "observability not
// configured", which is a legitimate default and stays silent.
var OnInvalidConfig func(component string, reason string)

// reportInvalidConfig notifies OnInvalidConfig when it is set.
func reportInvalidConfig(component, reason string) {
	if OnInvalidConfig == nil {
		return
	}
	OnInvalidConfig(component, reason)
}
