package outbound

import (
	"fmt"

	"github.com/yaling888/quirktiva/component/zivpn"
	"github.com/yaling888/quirktiva/constant"
)

type ZIVPN struct {
	*Socks5
	cleanup func()
}

type ZIVPNOption struct {
	Name           string `proxy:"name"`
	Server         string `proxy:"server"`
	Port           string `proxy:"port,omitempty"`
	Workers        int    `proxy:"workers,omitempty"`
	Password       string `proxy:"password,omitempty"`
	Obfs           string `proxy:"obfs,omitempty"`
	UDP            bool   `proxy:"udp,omitempty"`
	RecvWindowConn int    `proxy:"recv-window-conn,omitempty"`
	RecvWindow     int    `proxy:"recv-window,omitempty"`
	Up             string `proxy:"up,omitempty"`
	UpMbps         int    `proxy:"up-mbps,omitempty"`
	Down           string `proxy:"down,omitempty"`
	DownMbps       int    `proxy:"down-mbps,omitempty"`
}

func NewZIVPN(option ZIVPNOption) (*ZIVPN, error) {
	// Start ZIVPN Instance
	port, cleanup, err := zivpn.StartInstance(constant.Path.HomeDir(), option.Name, option.Server, option.Port, option.Password, option.Obfs, option.Workers, option.RecvWindowConn, option.RecvWindow, option.UDP, option.Up, option.UpMbps, option.Down, option.DownMbps)
	if err != nil {
		return nil, fmt.Errorf("zivpn start error: %w", err)
	}

	// Create internal Socks5 adapter pointing to the local port
	ssOption := Socks5Option{
		Name:             option.Name,
		Server:           "127.0.0.1",
		Port:             port,
		UDP:              option.UDP,
		RemoteDnsResolve: true,
	}

	ss, _ := NewSocks5(ssOption)
	ss.tp = constant.ZIVPN

	return &ZIVPN{
		Socks5:  ss,
		cleanup: cleanup,
	}, nil
}

func (z *ZIVPN) Cleanup() {
	if z.cleanup != nil {
		z.cleanup()
	}
	z.Socks5.Cleanup()
}
