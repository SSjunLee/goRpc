package registry

import (
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type LjnResigtery struct {
	timeout time.Duration
	mu      sync.Mutex
	servers map[string]*ServerItem
}

type ServerItem struct {
	Addr  string
	start time.Time
}

const (
	defaultPath    = "/_geerpc_/registry"
	defaultTimeout = time.Minute * 5
)

func New(timeout time.Duration) *LjnResigtery {
	return &LjnResigtery{
		servers: make(map[string]*ServerItem),
		timeout: timeout,
	}
}

var DefaultJnRegisterServer = New(defaultTimeout)

func (registry *LjnResigtery) putServer(addr string) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	r := registry.servers[addr]
	if r == nil {
		registry.servers[addr] = &ServerItem{addr, time.Now()}
	} else {
		r.start = time.Now()
	}
}

func (registry *LjnResigtery) aliveServers() []string {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	var alive []string
	for addr, server := range registry.servers {
		if registry.timeout == 0 || server.start.Add(registry.timeout).After(time.Now()) {
			alive = append(alive, addr)
		} else {
			delete(registry.servers, addr)
		}
	}
	sort.Strings(alive)
	return alive
}
func (registry *LjnResigtery) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case "GET":
		w.Header().Set("X-Geerpc-Servers", strings.Join(registry.aliveServers(), ","))
		//w.Write([]byte("hehehe"))
	case "POST":
		//心跳包处理
		addr := req.Header.Get("X-Geerpc-Server")
		if addr == "" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		registry.putServer(addr)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}

}
func (registry *LjnResigtery) HandleHttp(registryPath string) {
	http.Handle(registryPath, registry)
	log.Println("rpc registry path:", registryPath)
}

func HandleHttp() {
	DefaultJnRegisterServer.HandleHttp(defaultPath)
}

func HeartBeat(registry, addr string, duration time.Duration) {
	if duration == 0 {
		duration = defaultTimeout - time.Duration(1)*time.Minute
	}
	var err error
	err = sendHearBeat(registry, addr)
	go func() {
		t := time.NewTicker(duration)
		for err == nil {
			<-t.C
			log.Println("发心跳")
			err = sendHearBeat(registry, addr)
		}
	}()
}
func sendHearBeat(registry, addr string) error {
	log.Println(addr, "send heart beat to registry", registry)
	httpCLient := &http.Client{}
	req, _ := http.NewRequest("POST", registry, nil)
	req.Header.Set("X-Geerpc-Server", addr)
	httpCLient.Do(req)
	if _, err := httpCLient.Do(req); err != nil {
		log.Println("rpc server: heart beat err:", err)
		return err
	}
	return nil
}
