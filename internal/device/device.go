package device

import (
	"sync"

	"github.com/denisbrodbeck/machineid"
)

var (
	clientID string
	once     sync.Once
	initErr  error
)

// GetClientID 返回当前设备的唯一标识（延迟初始化，只计算一次）
func GetClientID() (string, error) {
	once.Do(func() {
		clientID, initErr = machineid.ProtectedID("WillClaw")
	})
	return clientID, initErr
}
