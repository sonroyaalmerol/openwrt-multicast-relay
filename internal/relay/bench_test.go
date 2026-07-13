package relay

import (
	"encoding/binary"
	"io"
	"log/slog"
	"net"
	"testing"
)

func buildTestMDNSPacket() []byte {
	ipHdr := make([]byte, 20)
	ipHdr[0] = 0x45
	ipHdr[2] = 0x00
	ipHdr[3] = 0x1f
	ipHdr[6] = 0x40
	ipHdr[8] = 255
	ipHdr[9] = 17
	copy(ipHdr[12:16], net.ParseIP("192.168.1.100").To4())
	copy(ipHdr[16:20], net.ParseIP("224.0.0.251").To4())

	udpHdr := make([]byte, 8)
	binary.BigEndian.PutUint16(udpHdr[0:2], 5353)
	binary.BigEndian.PutUint16(udpHdr[2:4], 5353)
	binary.BigEndian.PutUint16(udpHdr[4:6], 15)
	binary.BigEndian.PutUint16(udpHdr[6:8], 0)

	payload := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x04, 0x74, 0x65, 0x73, 0x74}

	pkt := append(ipHdr, udpHdr...)
	pkt = append(pkt, payload...)

	chk := computeIPChecksum(pkt[:20])
	binary.BigEndian.PutUint16(pkt[10:12], chk)

	return pkt
}

func buildTestTransmitter() transmitter {
	mac, _ := net.ParseMAC("aa:bb:cc:dd:ee:ff")
	return transmitter{
		relayAddr: MDNSMcastAddr,
		relayPort: MDNSMcastPort,
		iface:     "br-lan",
		ifindex:   1,
		ip:        "192.168.1.1",
		mac:       mac,
		netmask:   "255.255.255.0",
		broadcast: "192.168.1.255",
		fd:        -1,
		service:   "mDNS",
	}
}

func BenchmarkComputeIPChecksum(b *testing.B) {
	data := make([]byte, 20)
	data[0] = 0x45
	copy(data[12:16], []byte{192, 168, 1, 1})
	copy(data[16:20], []byte{224, 0, 0, 251})
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		computeIPChecksum(data)
	}
}

func BenchmarkComputeUDPChecksum(b *testing.B) {
	ipHeader := make([]byte, 20)
	copy(ipHeader[12:16], []byte{192, 168, 1, 1})
	copy(ipHeader[16:20], []byte{224, 0, 0, 251})
	payload := make([]byte, 100)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		computeUDPChecksum(ipHeader, payload, 5353, 5353)
	}
}

func BenchmarkIP4Str(b *testing.B) {
	ip := []byte{192, 168, 1, 100}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = ip4Str(ip)
	}
}

func BenchmarkOnNetworkIP(b *testing.B) {
	src := []byte{192, 168, 1, 100}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		onNetworkIP(src, "192.168.1.1", "255.255.255.0")
	}
}

func BenchmarkBytesEqual4(b *testing.B) {
	a := []byte{192, 168, 1, 1}
	c := []byte{192, 168, 1, 2}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		bytesEqual4(a, c)
	}
}

func BenchmarkMulticastIPToMAC(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		multicastIPToMAC("239.255.255.250")
	}
}

func BenchmarkHandlePacket(b *testing.B) {
	pkt := buildTestMDNSPacket()

	r := &Relay{
		cfg: &Config{
			Interfaces: []string{"192.168.1.0/24", "192.168.3.0/24"},
		},
		allIfaces: []transmitter{
			buildTestTransmitter(),
			{
				relayAddr: MDNSMcastAddr,
				relayPort: MDNSMcastPort,
				iface:     "br-iot",
				ifindex:   2,
				ip:        "192.168.3.1",
				netmask:   "255.255.255.0",
			},
		},
		transmitters:   []transmitter{buildTestTransmitter()},
		bindings:       map[[2]string]bool{{MDNSMcastAddr, "5353"}: true},
		recentChecksum: make([]uint16, 256),
		cksumIdx:       0,
	}
	mac, _ := net.ParseMAC("01:00:5e:00:00:fb")
	r.etherAddrs = map[string]net.HardwareAddr{MDNSMcastAddr: mac}
	r.nif = newNetifaces()
	r.logger = nopLogger()

	recentSSDP := make(map[string]uint16)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		r.cksumIdx = 0
		r.handlePacket(pkt, "local", &recentSSDP)
	}
}

func nopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestHandlePacketZeroAlloc(t *testing.T) {
	pkt := buildTestMDNSPacket()

	r := &Relay{
		cfg: &Config{
			Interfaces: []string{"192.168.1.0/24", "192.168.3.0/24"},
		},
		allIfaces: []transmitter{
			buildTestTransmitter(),
			{
				relayAddr: MDNSMcastAddr,
				relayIP:   ipToBytes(MDNSMcastAddr),
				relayPort: MDNSMcastPort,
				iface:     "br-iot",
				ifindex:   2,
				ip:        "192.168.3.1",
				netmask:   "255.255.255.0",
			},
		},
		transmitters:   []transmitter{buildTestTransmitter()},
		recentChecksum: make([]uint16, 256),
		cksumIdx:       0,
	}
	mac, _ := net.ParseMAC("01:00:5e:00:00:fb")
	r.etherAddrs = map[string]net.HardwareAddr{MDNSMcastAddr: mac}
	r.nif = newNetifaces()
	r.logger = nopLogger()

	recentSSDP := make(map[string]uint16)

	pktCopy := make([]byte, len(pkt))

	allocs := testing.AllocsPerRun(100, func() {
		copy(pktCopy, pkt)
		r.cksumIdx = 0
		r.handlePacket(pktCopy, "local", &recentSSDP)
	})

	if allocs != 0 {
		t.Errorf("handlePacket allocs = %v, want 0", allocs)
	}
}

func TestComputeIPChecksumZeroAlloc(t *testing.T) {
	data := make([]byte, 20)
	data[0] = 0x45
	copy(data[12:16], []byte{192, 168, 1, 1})
	copy(data[16:20], []byte{224, 0, 0, 251})

	allocs := testing.AllocsPerRun(100, func() {
		_ = computeIPChecksum(data)
	})

	if allocs != 0 {
		t.Errorf("computeIPChecksum allocs = %v, want 0", allocs)
	}
}

func TestComputeUDPChecksumZeroAlloc(t *testing.T) {
	ipHeader := make([]byte, 20)
	copy(ipHeader[12:16], []byte{192, 168, 1, 1})
	copy(ipHeader[16:20], []byte{224, 0, 0, 251})
	payload := make([]byte, 100)

	allocs := testing.AllocsPerRun(100, func() {
		_ = computeUDPChecksum(ipHeader, payload, 5353, 5353)
	})

	if allocs != 0 {
		t.Errorf("computeUDPChecksum allocs = %v, want 0", allocs)
	}
}

func TestOnNetworkIPZeroAlloc(t *testing.T) {
	src := []byte{192, 168, 1, 100}

	allocs := testing.AllocsPerRun(100, func() {
		_ = onNetworkIP(src, "192.168.1.1", "255.255.255.0")
	})

	if allocs != 0 {
		t.Errorf("onNetworkIP allocs = %v, want 0", allocs)
	}
}

func TestBytesEqual4ZeroAlloc(t *testing.T) {
	a := []byte{192, 168, 1, 1}
	b := []byte{192, 168, 1, 2}

	allocs := testing.AllocsPerRun(100, func() {
		_ = bytesEqual4(a, b)
	})

	if allocs != 0 {
		t.Errorf("bytesEqual4 allocs = %v, want 0", allocs)
	}
}
