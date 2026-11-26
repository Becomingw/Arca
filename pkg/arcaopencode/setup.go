package arcaopencode

import _ "embed"

//go:embed sync.py
var syncScript string

// SyncScript returns the Linux/macOS integration script without credentials.
func SyncScript() string {
	return syncScript
}
