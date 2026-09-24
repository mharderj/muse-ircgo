package irc

import "log"

// Debug enables connection-stage diagnostics on the standard logger. The
// application wires the logger's output (e.g. to debug.log via -debug).
// Secrets are never logged: PASS payloads and SASL blobs are redacted at
// the call sites that would emit them.
var Debug bool

func debugf(server, format string, args ...any) {
	if !Debug {
		return
	}
	if server == "" {
		log.Printf("[ircgo] "+format, args...)
		return
	}
	log.Printf("[ircgo "+server+"] "+format, args...)
}
