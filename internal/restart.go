package internal

import "sync/atomic"

var restartRequested int64

// MARK: SetRestartFlag
func SetRestartFlag(restart bool) {
	if restart {
		atomic.StoreInt64(&restartRequested, 1)
	} else {
		atomic.StoreInt64(&restartRequested, 0)
	}
}

// MARK: ShouldRestart
func ShouldRestart() bool {
	return atomic.LoadInt64(&restartRequested) == 1
}
