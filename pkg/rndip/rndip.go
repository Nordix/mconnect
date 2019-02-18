package rndip

import (
	"fmt"
	"math/rand"
	"net"
)

type Rndip struct {
	baseCidr string
	net      *net.IPNet
}

func New(baseCidr string) (*Rndip, error) {
	_, anet, err := net.ParseCIDR(baseCidr)
	if err != nil {
		return nil, err
	}
	return &Rndip{
		baseCidr: baseCidr,
		net:      anet,
	}, nil
}

func (r *Rndip) GetNet() *net.IPNet {
	return r.net
}

func (r *Rndip) GetIP() net.IP {
	if r.net.IP.To4() != nil {
		ip := make([]byte, 4)
		rand.Read(ip)
		for i := 0; i < 4; i++ {
			ip[i] = r.net.IP[i]&r.net.Mask[i] + ip[i]&^r.net.Mask[i]
		}
		return ip
	}
	ip := make([]byte, 16)
	rand.Read(ip)
	for i := 0; i < 16; i++ {
		ip[i] = r.net.IP[i]&r.net.Mask[i] + ip[i]&^r.net.Mask[i]
	}
	return ip
}

func (r *Rndip) GetIPString() string {
	ip := r.GetIP()
	if ip.To4() == nil {
		return fmt.Sprintf("[%v]", ip)
	}
	return fmt.Sprintf("%v", ip)
}
