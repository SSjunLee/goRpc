package xclient

import (
	"log"
	"net/http"
	"strings"
	"time"
)

type GeeRegisterDiscovery struct {
	*MultiServerDiscovery
	registry   string
	timeout    time.Duration
	lastUpdate time.Time
}

const DefaultUpdateTime = time.Second * 10

func NewGeeRegisterDiscovery(registry string, timeout time.Duration) *GeeRegisterDiscovery {
	return &GeeRegisterDiscovery{
		registry:             registry,
		timeout:              timeout,
		MultiServerDiscovery: NewMultiServerDiscovery(make([]string, 0)),
	}
}

func (d *GeeRegisterDiscovery) Refresh() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.lastUpdate.Add(d.timeout).After(time.Now()) {
		return nil
	}
	log.Println("rpc registry: refresh servers from registry", d.registry)
	rep, err := http.Get(d.registry)
	if err != nil {
		log.Println("rpc registry refresh err:", err)
		return err
	}
	servers := strings.Split(rep.Header.Get("X-Geerpc-Servers"), ",")
	d.servers = make([]string, 0, len(servers))
	for _, server := range servers {
		if strings.TrimSpace(server) != "" {
			servers = append(servers, strings.TrimSpace(server))
		}
	}
	d.lastUpdate = time.Now()
	return nil
}
func (d *GeeRegisterDiscovery) Update(servers []string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.servers = servers
	d.lastUpdate = time.Now()
	return nil
}

func (d *GeeRegisterDiscovery) Get(mode SelectMode) (string, error) {
	if err := d.Refresh(); err != nil {
		return "", err
	}
	return d.MultiServerDiscovery.Get(mode)
}
func (d *GeeRegisterDiscovery) GetAll() ([]string, error) {
	if err := d.Refresh(); err != nil {
		return nil, err
	}
	return d.MultiServerDiscovery.GetAll()
}
