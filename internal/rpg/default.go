package rpg

import "sync/atomic"

// Initialized once during bot startup; nil means RPG is disabled.
var defaultService atomic.Pointer[Service]

func SetDefault(s *Service) { defaultService.Store(s) }
func Default() *Service     { return defaultService.Load() }
