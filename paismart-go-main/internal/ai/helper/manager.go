package helper

import (
	"sync"
)

// AIHelperFactory 定义创建 AIHelper 的工厂能力。
// 这个和默认工厂分离的好处是可以在测试中 mock 这个工厂，或者在未来需要不同创建逻辑时实现不同的工厂。
type AIHelperFactory interface {
	Create(userID uint, conversationID string) (*AIHelper, error)
}

// Manager 管理用户与会话对应的 AIHelper 实例。
// 建议使用：map[userID]map[conversationID]*AIHelper
// 为什么使用双层 map？因为一个用户可能有多个会话（比如多设备登录），每个会话对应一个 AIHelper。
// 第一层 map 以 userID 为键，第二层 map 以 conversationID 为键，这样可以快速定位到对应的 AIHelper 实例。
// 这种设计会增加内存使用，但可以显著提高查找效率，尤其是在用户和会话数量较多的情况下。在海量aihelper的情况下会出现问题，
// 后续可以考虑使用 LRU 缓存或者其他淘汰策略来限制内存使用。
// 是否需要修改双层map为单层map？单层 map 以 "userID:conversationID" 作为键，这样可以简化代码，但在查找时需要进行字符串拼接和解析，
// 可能会稍微降低性能。双层 map 的设计更清晰，易于维护和理解，尤其是在需要频繁访问用户或会话相关数据的场景下。
type Manager struct {
	// 为什么是 RWMutex？因为 Get 操作是读操作，GetOrCreate 和 Remove 是写操作。使用 RWMutex 可以允许多个读操作并行执行，
	// 提高性能，同时保证写操作的安全性。
	// 如果是Mutex，那么所有操作都需要排队执行，可能会导致性能瓶颈，尤其是在高并发场景下。
	mu      sync.RWMutex
	helpers map[uint]map[string]*AIHelper
	factory AIHelperFactory
}

// NewManager 创建一个新的 AIHelper Manager。
func NewManager(factory AIHelperFactory) *Manager {
	return &Manager{
		helpers: make(map[uint]map[string]*AIHelper),
		factory: factory,
	}
}

// Get 返回一个已存在的 helper，不存在则返回 nil。
func (m *Manager) Get(userID uint, conversationID string) *AIHelper {
	m.mu.RLock()
	defer m.mu.RUnlock()

	userHelpers, ok := m.helpers[userID]
	if !ok {
		return nil
	}

	return userHelpers[conversationID]
}

// GetOrCreate 获取或创建一个 helper。
func (m *Manager) GetOrCreate(userID uint, conversationID string) (*AIHelper, error) {
	// fast path
	m.mu.RLock()
	userHelpers, ok := m.helpers[userID]
	if ok {
		if helper, exists := userHelpers[conversationID]; exists {
			m.mu.RUnlock()
			return helper, nil
		}
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	// double check
	// 为什么需要 double check？因为在获取锁之前，可能有其他 goroutine 已经创建了 helper，所以在获取锁之后需要再次检查是否已经存在。
	// 这是最常见的 double check 场景，可以避免不必要的创建和锁竞争，提高性能。
	// 还有其他场景需要 double check 吗？比如在单例模式中，获取实例之前也需要 double check，以确保线程安全。
	// double check 的缺点是什么？增加了代码复杂度，可能会导致错误的实现（比如忘记第二次检查），需要谨慎使用。
	userHelpers, ok = m.helpers[userID]
	if !ok {
		userHelpers = make(map[string]*AIHelper)
		m.helpers[userID] = userHelpers
	}

	if helper, exists := userHelpers[conversationID]; exists {
		return helper, nil
	}

	helper, err := m.factory.Create(userID, conversationID)
	if err != nil {
		return nil, err
	}

	userHelpers[conversationID] = helper
	return helper, nil
}

// Remove 删除指定会话的 helper。
func (m *Manager) Remove(userID uint, conversationID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	userHelpers, ok := m.helpers[userID]
	if !ok {
		return
	}

	delete(userHelpers, conversationID)

	if len(userHelpers) == 0 {
		delete(m.helpers, userID)
	}
}

// RemoveUser 删除某个用户下的所有 helper。
func (m *Manager) RemoveUser(userID uint) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.helpers, userID)
}
