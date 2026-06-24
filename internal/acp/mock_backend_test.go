package acp

import "time"

// MockBackend 是测试用的 Backend 实现，启动一个简单的 echo 进程。
type MockBackend struct {
	name    string
	command string
	args    []string
	envs    []string
	timeout time.Duration
}

func NewMockBackend() *MockBackend {
	return &MockBackend{
		name:    "mock",
		command: "echo",
		args:    []string{"mock-agent"},
		timeout: 10 * time.Second,
	}
}

func (b *MockBackend) Name() string           { return b.name }
func (b *MockBackend) Command() string        { return b.command }
func (b *MockBackend) Args() []string         { return b.args }
func (b *MockBackend) Env() []string          { return b.envs }
func (b *MockBackend) Timeout() time.Duration { return b.timeout }
