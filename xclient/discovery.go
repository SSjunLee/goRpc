package xclient

import (
	"errors"
	"math"
	"math/rand"
	"sync"
	"time"
)

type SelectMode int

const (
	RandomSelect SelectMode = iota
	RondRobinSelect
)

type Discovery interface {
	Refresh() error                      //从注册中心更新服务列表
	Update([]string) error               //手动更新服务列表
	Get(mode SelectMode) (string, error) //根据负载均衡策略，选择一个服务实例
	GetAll() ([]string, error)           //返回所有的服务实例
}

//不需要注册中心，服务列表由手工维护的服务发现的结构体
type MultiServerDiscovery struct {
	r       *rand.Rand //随机数种子
	mu      sync.Mutex
	servers []string //服务列表
	index   int      //当前访问的服务索引
}

func NewMultiServerDiscovery(servers []string) *MultiServerDiscovery {
	res := &MultiServerDiscovery{
		servers: servers,
		r:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	res.index = res.r.Intn(math.MaxInt32 - 1)
	return res
}

func (d *MultiServerDiscovery) Refresh() error {
	return nil
}
func (d *MultiServerDiscovery) Update(servers []string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.servers = servers
	return nil
}
func (d *MultiServerDiscovery) Get(mode SelectMode) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := len(d.servers)
	if n == 0 {
		return "", errors.New("rpc discovery: no available servers")
	}
	switch mode {
	case RandomSelect:
		return d.servers[d.r.Intn(n)], nil
	case RondRobinSelect:
		ret := d.servers[d.index%n]
		d.index = (d.index + 1) % n
		return ret, nil
	default:
		return "", errors.New("rpc discovery: not supported select mode")
	}
}
func (d *MultiServerDiscovery) GetAll() ([]string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	servers := make([]string, len(d.servers), len(d.servers))
	copy(servers, d.servers)
	return servers, nil
}
