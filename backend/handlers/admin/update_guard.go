package admin

import "sync"

var updateOperationGuard = struct {
	sync.Mutex
	active map[string]bool
}{
	active: make(map[string]bool),
}

func tryBeginUpdateOperation(target string) bool {
	updateOperationGuard.Lock()
	defer updateOperationGuard.Unlock()

	if updateOperationGuard.active[target] {
		return false
	}
	updateOperationGuard.active[target] = true
	return true
}

func endUpdateOperation(target string) {
	updateOperationGuard.Lock()
	defer updateOperationGuard.Unlock()

	delete(updateOperationGuard.active, target)
}
