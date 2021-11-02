package main

import (
	"net"
	"testing"
)

func TestGenerateIPAddr(t *testing.T) {
	_, v4Net, _ := net.ParseCIDR("127.10.0.0/16")
	_, v6Net, _ := net.ParseCIDR("fe80::/112")
	{
		ipv4, ipv6, err := generateIPAddr(v4Net, v6Net, 100)
		if err != nil {
			t.Error("Failed to generate IP: ", err)
		}
		if expected := "127.10.0.100"; ipv4.String() != expected {
			t.Errorf("Failed to generate IPv4: %s(expected) != %s(actual)", ipv4.String(), expected)
		}
		if expected := "fe80::100"; ipv6.String() != expected {
			t.Errorf("Failed to generate IPv6: %s(expected) != %s(actual)", ipv6.String(), expected)
		}
	}
	_, ipv6, err := generateIPAddr(v4Net, v6Net, 256)
	if err == nil {
		t.Errorf("%s contain only 255 valid v6 address, but got: %s", v6Net.String(), ipv6.String())
	}

	_, v4Net, _ = net.ParseCIDR("127.10.10.128/25")
	_, v6Net, _ = net.ParseCIDR("fe80::/64")
	ipv4, _, err := generateIPAddr(v4Net, v6Net, 129)
	if err == nil {
		t.Errorf("%s contain only 126 valid v4 address, but got: %s", v4Net.String(), ipv4.String())
	}

}

func testCalcDefaultGateway(t *testing.T, validCIDR, expectedGateway string, invalidCIDR string) {
	{
		ip, network, err := calcDefaultGateway(validCIDR)
		if err != nil {
			t.Error(err)
		}
		if !ip.Equal(net.ParseIP(expectedGateway)) {
			t.Errorf("Default gateway of %s must be %s, but got %s (in %s)", validCIDR, expectedGateway, ip.String(), network.String())
		}
	}
	{
		if ip, network, err := calcDefaultGateway(invalidCIDR); err == nil {
			t.Errorf("There should not be default GW for %s, but got %s(in %s)", invalidCIDR, ip.String(), network.String())
		}
	}

}

func TestCalcDefaultGatewayV6(t *testing.T) {
	testCalcDefaultGateway(t, "fe80:1234:1234:1234::/64", "fe80:1234:1234:1234::1", "fe80:1234:1234:1234::/128")
}
func TestCalcDefaultGatewayV4(t *testing.T) {
	testCalcDefaultGateway(t, "127.168.128.0/18", "127.168.128.1", "127.168.128.0/32")
}
