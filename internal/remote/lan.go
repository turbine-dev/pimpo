package remote

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

// LAN serves the web app on the home network's address as well, so a
// phone on the same Wi-Fi reaches Vigia with no account at all. Only
// private addresses are used: nothing is exposed outside the house.
type LAN struct {
	Port    int
	Handler http.Handler
	// Addr finds the machine's address on the home network; tests replace it.
	Addr func() (net.IP, error)

	mu  sync.Mutex
	srv *http.Server
	url string
}

// HomeAddress returns the first private IPv4 address of an interface
// that is up.
func HomeAddress() (net.IP, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for _, i := range ifaces {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil && n.IP.IsPrivate() {
				return n.IP.To4(), nil
			}
		}
	}
	return nil, errors.New("this computer is not on a home network")
}

func (l *LAN) Start() (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.srv != nil {
		return l.url, nil
	}
	find := l.Addr
	if find == nil {
		find = HomeAddress
	}
	ip, err := find()
	if err != nil {
		return "", err
	}
	if !ip.IsPrivate() {
		return "", fmt.Errorf("%s is not a home network address", ip)
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(ip.String(), fmt.Sprint(l.Port)))
	if err != nil {
		return "", fmt.Errorf("could not listen on the home network: %w", err)
	}
	l.srv = &http.Server{Handler: l.Handler, ReadHeaderTimeout: 10 * time.Second}
	l.url = "http://" + ln.Addr().String()
	go l.srv.Serve(ln)
	return l.url, nil
}

func (l *LAN) URL() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.url
}

func (l *LAN) Stop() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.srv != nil {
		l.srv.Close()
	}
	l.srv, l.url = nil, ""
}
