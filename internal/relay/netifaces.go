package relay

import (
	"fmt"
	"net"
)

type ifInfo struct {
	Name      string
	MAC       net.HardwareAddr
	IP        string
	Netmask   string
	Broadcast string
}

type netifaces struct {
	ifaces map[string]ifInfo
}

func newNetifaces() *netifaces {
	return &netifaces{ifaces: make(map[string]ifInfo)}
}

func (n *netifaces) Discover() error {
	netIfaces, err := net.Interfaces()
	if err != nil {
		return fmt.Errorf("list interfaces: %w", err)
	}

	for _, iface := range netIfaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok || ipNet.IP.To4() == nil {
				continue
			}

			mask := ipNet.Mask
			maskStr := fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3])
			ipLong := ipToUint32(ipNet.IP.String())
			maskLong := ipToUint32(maskStr)
			bcast := uint32ToIP(ipLong | ^maskLong).String()

			mac := iface.HardwareAddr
			if len(mac) == 0 {
				mac = net.HardwareAddr{0, 0, 0, 0, 0, 0}
			}

			info := ifInfo{
				Name:      iface.Name,
				MAC:       mac,
				IP:        ipNet.IP.String(),
				Netmask:   maskStr,
				Broadcast: bcast,
			}
			n.ifaces[iface.Name] = info
			n.ifaces[ipNet.IP.String()] = info

			ones, _ := ipNet.Mask.Size()
			cidr := fmt.Sprintf("%s/%d", ipNet.IP.String(), ones)
			n.ifaces[cidr] = info
		}
	}
	return nil
}

func (n *netifaces) Resolve(spec string) (ifInfo, error) {
	if info, ok := n.ifaces[spec]; ok {
		return info, nil
	}
	return ifInfo{}, fmt.Errorf("interface %s does not exist", spec)
}
