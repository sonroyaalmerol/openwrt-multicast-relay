package relay

import (
	"encoding/binary"
	"net"
	"os"
	"strings"
)

const (
	MulticastMin    = "224.0.0.0"
	MulticastMax    = "239.255.255.255"
	Broadcast       = "255.255.255.255"
	SSDPMcastAddr   = "239.255.255.250"
	SSDPMcastPort   = 1900
	SSDPUnicastPort = 1901
	MDNSMcastAddr   = "224.0.0.251"
	MDNSMcastPort   = 5353
	magicLen        = 4
	udpMaxLength    = 1458
)

func ipToUint32(ip string) uint32 {
	return binary.BigEndian.Uint32(net.ParseIP(ip).To4())
}

func bytesEqual4(a, b []byte) bool {
	return len(a) >= 4 && len(b) >= 4 &&
		a[0] == b[0] && a[1] == b[1] && a[2] == b[2] && a[3] == b[3]
}

func onNetworkIP(srcIP []byte, network, netmask string) bool {
	netIP := net.ParseIP(network).To4()
	mask := net.ParseIP(netmask).To4()
	if len(netIP) < 4 || len(mask) < 4 || len(srcIP) < 4 {
		return false
	}
	return (srcIP[0]&mask[0]) == (netIP[0]&mask[0]) &&
		(srcIP[1]&mask[1]) == (netIP[1]&mask[1]) &&
		(srcIP[2]&mask[2]) == (netIP[2]&mask[2]) &&
		(srcIP[3]&mask[3]) == (netIP[3]&mask[3])
}

func uint32ToIP(n uint32) net.IP {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, n)
	return net.IP(b)
}

func isMulticast(ip string) bool {
	v := ipToUint32(ip)
	return v >= ipToUint32(MulticastMin) && v <= ipToUint32(MulticastMax)
}

func isBroadcast(ip string) bool {
	return ip == Broadcast
}

func onNetwork(ip, network, netmask string) bool {
	return (ipToUint32(ip) & ipToUint32(netmask)) == (ipToUint32(network) & ipToUint32(netmask))
}

func cidrToNetmask(bits int) string {
	mask := uint32((1<<32 - 1) << (32 - bits))
	return uint32ToIP(mask).String()
}

func multicastIPToMAC(addr string) net.HardwareAddr {
	v := ipToUint32(addr) & 0x007fffff
	mac := uint64(0x01005e000000) | uint64(v)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, mac)
	return buf[2:]
}

func broadcastIPToMAC() net.HardwareAddr {
	return net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
}

func computeIPChecksum(data []byte) uint16 {
	var sum uint32
	for i := 0; i+1 < len(data); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(data[i : i+2]))
	}
	if len(data)%2 != 0 {
		sum += uint32(data[len(data)-1]) << 8
	}
	for sum > 0xffff {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum)
}

func computeUDPChecksum(ipHeader, data []byte, srcPort, dstPort uint16) uint16 {
	var sum uint32

	for i := 0; i+1 < 8; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(ipHeader[12+i:][:2]))
	}

	udpLen := uint16(8 + len(data))
	sum += uint32(17)
	sum += uint32(udpLen)
	sum += uint32(srcPort)
	sum += uint32(dstPort)
	sum += uint32(udpLen)

	for i := 0; i+1 < len(data); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(data[i:][:2]))
	}
	if len(data)%2 != 0 {
		sum += uint32(data[len(data)-1]) << 8
	}

	for sum > 0xffff {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum)
}

func modifyUDPPacket(data []byte, ipHdrLen int, srcAddr, dstAddr net.IP, srcPort, dstPort *uint16) []byte {
	out := make([]byte, len(data))
	copy(out, data)
	if srcAddr != nil {
		copy(out[12:16], srcAddr.To4())
	}
	if dstAddr != nil {
		copy(out[16:20], dstAddr.To4())
	}
	if srcPort != nil {
		binary.BigEndian.PutUint16(out[ipHdrLen:ipHdrLen+2], *srcPort)
	}
	if dstPort != nil {
		binary.BigEndian.PutUint16(out[ipHdrLen+2:ipHdrLen+4], *dstPort)
	}
	return out
}

func mdnsSetUnicastBit(data []byte, ipHdrLen int) []byte {
	udpData := data[ipHdrLen+8:]
	flags := binary.BigEndian.Uint16(udpData[2:4])
	if flags&0x8000 != 0 {
		return data
	}

	queries := binary.BigEndian.Uint16(udpData[4:6])
	queryCount := uint16(0)
	ptr := 12

	out := make([]byte, len(udpData))
	copy(out, udpData)

	for ptr < len(out) {
		labelLen := out[ptr]
		if labelLen&0xc0 == 0xc0 {
			queryCount++
			binary.BigEndian.PutUint16(out[ptr+3:ptr+5], binary.BigEndian.Uint16(out[ptr+3:ptr+5])|0x8000)
			if queryCount == queries {
				break
			}
			ptr += 5
		} else if labelLen&0x3f == 0 {
			queryCount++
			binary.BigEndian.PutUint16(out[ptr+3:ptr+5], binary.BigEndian.Uint16(out[ptr+3:ptr+5])|0x8000)
			if queryCount == queries {
				break
			}
			ptr += 5
		} else {
			ptr += int(labelLen) + 1
		}
	}

	result := make([]byte, len(data))
	copy(result, data[:ipHdrLen+8])
	copy(result[ipHdrLen+8:], out)
	return result
}

func unicastIPToMAC(ip string) (net.HardwareAddr, error) {
	data, err := os.ReadFile("/proc/net/arp")
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(string(data), "\n")[1:] {
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[0] != ip {
			continue
		}
		if fields[3] == "00:00:00:00:00:00" {
			continue
		}
		mac, err := net.ParseMAC(fields[3])
		if err != nil {
			continue
		}
		return mac, nil
	}
	return nil, nil
}
