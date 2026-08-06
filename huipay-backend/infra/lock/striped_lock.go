// 包 lock 提供进程内的条带锁（256 桶 sync.Mutex）。
// P3 之前不引入 Redis；单进程内用于分账/支付等关键链路的并发控制。
package lock

import (
	"hash/fnv"
	"sync"
)

const bucketCount = 256

// StripedLock 按 key 哈希分桶的细粒度锁。
type StripedLock struct {
	buckets [bucketCount]sync.Mutex
}

// New 构造条带锁。
func New() *StripedLock { return &StripedLock{} }

// Lock 获取锁，返回释放函数。
func (l *StripedLock) Lock(key string) func() {
	idx := fnv32(key) % bucketCount
	l.buckets[idx].Lock()
	return func() { l.buckets[idx].Unlock() }
}

func fnv32(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32()
}