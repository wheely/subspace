package main

import (
	"errors"
	"fmt"
	"net"
)

var (
	kErrIPLimitExceeds = errors.New("num of devices exceeds the limit of IP addr pool")
)

func cloneIP(ip net.IP) net.IP {
	dup := make(net.IP, len(ip))
	copy(dup, ip)
	return dup
}

func calcDefaultGateway(cidr string) (net.IP, *net.IPNet, error) {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, nil, err
	}
	if ones, bits := network.Mask.Size(); bits-ones == 0 {
		return nil, nil, fmt.Errorf("given CIDR (%s) doet not represent a network", cidr)
	}
	gwIP := cloneIP(network.IP)
	gwIP[len(gwIP)-1] |= 1
	return gwIP, network, nil
}

func generateIPAddr(v4Net *net.IPNet, v6Net *net.IPNet, id uint32) (net.IP, net.IP, error) {
	v4 := cloneIP(v4Net.IP)
	v6 := cloneIP(v6Net.IP)
	for left, pos4, pos6 := id, len(v4)-1, len(v6)-2; left != 0; left, pos4, pos6 = left>>8, pos4-1, pos6-2 {
		decimalId := byte(left & 0xff)
		v4[pos4] += decimalId
		hexId := uint16(decimalId%10) + uint16((decimalId/10)%10)*16 + uint16(decimalId/100)*256
		v6[pos6+0] += byte((hexId >> 8) & 0xff)
		v6[pos6+1] += byte(hexId & 0xff)
	}
	if !v4Net.Contains(v4) || v4.Equal(net.IPv4(0xff, 0xff, 0xff, 0xff).Mask(v4Net.Mask)) {
		return nil, nil, kErrIPLimitExceeds
	}
	if !v6Net.Contains(v6) || !(v6.IsGlobalUnicast() || v6.IsLinkLocalUnicast()) {
		return nil, nil, kErrIPLimitExceeds
	}
	return v4, v6, nil
}
