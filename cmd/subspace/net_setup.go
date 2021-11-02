package main

import (
	"net"
)

var (
	wgConfig *wireguardConfig
)

type wireguardConfig struct {
	gatewayIPv4 net.IP
	gatewayIPv6 net.IP
	networkIPv4 *net.IPNet
	networkIPv6 *net.IPNet
}

func initWireguardConfig() error {
	gatewayIPv4, networkIPv4, err := calcDefaultGateway(networkIPv4)
	if err != nil {
		return err
	}
	gatewayIPv6, networkIPv6, err := calcDefaultGateway(networkIPv6)
	if err != nil {
		return err
	}
	wgConfig = &wireguardConfig{
		gatewayIPv4: gatewayIPv4,
		networkIPv4: networkIPv4,
		gatewayIPv6: gatewayIPv6,
		networkIPv6: networkIPv6,
	}
	return nil
}

func (conf *wireguardConfig) generateIPAddr(id uint32) (net.IP, net.IP, error) {
	return generateIPAddr(conf.networkIPv4, conf.networkIPv6, id)
}
