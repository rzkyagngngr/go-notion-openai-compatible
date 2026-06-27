package browserrefresh

import "sync"

var globalMu sync.Mutex

func acquireLock() func() {
	globalMu.Lock()
	return globalMu.Unlock
}