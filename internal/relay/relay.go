package relay

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

const etherTypeIPv4 = 0x0800

var bufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 65535)
		return &b
	},
}

var pktPool = sync.Pool{
	New: func() any {
		b := make([]byte, 65535)
		return &b
	},
}

var txBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 65535)
		return &b
	},
}

func ip4Str(b []byte) string {
	return fmt.Sprintf("%d.%d.%d.%d", b[0], b[1], b[2], b[3])
}

type transmitter struct {
	relayAddr string
	relayPort int
	iface     string
	ifindex   int
	ip        string
	mac       net.HardwareAddr
	netmask   string
	broadcast string
	fd        int
	service   string
	rxOnly    bool
}

type receiver struct {
	fd       int
	etherHdr bool
}

type remoteConn struct {
	addr   string
	conn   *net.TCPConn
	failAt time.Time
}

type Relay struct {
	cfg    *Config
	logger *slog.Logger
	nif    *netifaces

	allIfaces    []transmitter
	transmitters []transmitter
	receivers    []receiver
	bindings     map[[2]string]bool
	etherAddrs   map[string]net.HardwareAddr

	recentChecksum []uint16
	cksumIdx       int

	listenSock  *net.TCPListener
	remoteAddrs []remoteConn
	remoteConns []*net.TCPConn
	remoteCh    chan []byte
	aes         *aesCipher
}

func New(cfg *Config) (*Relay, error) {
	logLevel := slog.LevelWarn
	if cfg.Verbose {
		logLevel = slog.LevelInfo
	}

	var handlers []slog.Handler
	if cfg.Foreground {
		handlers = append(handlers, slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
	}
	if cfg.Logfile != "" {
		f, err := os.OpenFile(cfg.Logfile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, fmt.Errorf("open logfile: %w", err)
		}
		handlers = append(handlers, slog.NewTextHandler(f, &slog.HandlerOptions{Level: logLevel}))
	}
	var handler slog.Handler
	if len(handlers) == 0 {
		handler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})
	} else if len(handlers) == 1 {
		handler = handlers[0]
	} else {
		handler = slog.NewMultiHandler(handlers...)
	}
	logger := slog.New(handler)

	nif := newNetifaces()
	if err := nif.Discover(); err != nil {
		return nil, fmt.Errorf("discover interfaces: %w", err)
	}

	c, err := newCipher(cfg.AESKey)
	if err != nil {
		return nil, fmt.Errorf("init cipher: %w", err)
	}

	r := &Relay{
		cfg:        cfg,
		logger:     logger,
		nif:        nif,
		bindings:   make(map[[2]string]bool),
		etherAddrs: make(map[string]net.HardwareAddr),
		aes:        c,
		remoteCh:   make(chan []byte, 256),

		recentChecksum: make([]uint16, 256),
	}

	relays := cfg.DefaultRelays()
	if err := cfg.ValidateRelays(relays); err != nil {
		return nil, err
	}

	for _, spec := range relays {
		if err := r.addListener(spec.Addr, spec.Port, spec.Service); err != nil {
			return nil, fmt.Errorf("add listener %s:%d: %w", spec.Addr, spec.Port, err)
		}
	}

	if len(cfg.ListenAddrs) > 0 {
		ln, err := net.ListenTCP("tcp", &net.TCPAddr{Port: cfg.RemotePort})
		if err != nil {
			return nil, fmt.Errorf("listen on %d: %w", cfg.RemotePort, err)
		}
		r.listenSock = ln
	} else if len(cfg.RemoteAddrs) > 0 {
		r.remoteAddrs = make([]remoteConn, len(cfg.RemoteAddrs))
		for i, addr := range cfg.RemoteAddrs {
			r.remoteAddrs[i] = remoteConn{addr: addr}
		}
		r.connectRemotes()
	}

	return r, nil
}

func (r *Relay) addListener(addr string, port int, service string) error {
	if isBroadcast(addr) {
		r.etherAddrs[addr] = broadcastIPToMAC()
	} else if isMulticast(addr) {
		r.etherAddrs[addr] = multicastIPToMAC(addr)
	} else {
		r.etherAddrs[addr] = nil
		fd, err := unix.Socket(unix.AF_INET, unix.SOCK_RAW, unix.IPPROTO_UDP)
		if err != nil {
			return fmt.Errorf("create raw socket: %w", err)
		}
		if err := unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEADDR, 1); err != nil {
			_ = unix.Close(fd)
			return fmt.Errorf("setsockopt reuseaddr: %w", err)
		}
		ip4 := net.ParseIP(addr).To4()
		var sa unix.SockaddrInet4
		copy(sa.Addr[:], ip4)
		sa.Port = port
		if err := unix.Bind(fd, &sa); err != nil {
			_ = unix.Close(fd)
			return fmt.Errorf("bind raw socket: %w", err)
		}
		r.receivers = append(r.receivers, receiver{fd: fd})
	}

	if isMulticast(addr) {
		fd, err := unix.Socket(unix.AF_INET, unix.SOCK_RAW, unix.IPPROTO_UDP)
		if err != nil {
			return fmt.Errorf("create mcast socket: %w", err)
		}
		if err := unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEADDR, 1); err != nil {
			_ = unix.Close(fd)
			return fmt.Errorf("setsockopt reuseaddr: %w", err)
		}

		parsedIP := net.ParseIP(addr).To4()
		for _, ifaceSpec := range r.cfg.Interfaces {
			info, err := r.nif.Resolve(ifaceSpec)
			if err != nil {
				return err
			}

			if isBroadcast(addr) {
				bfd, err := unix.Socket(unix.AF_INET, unix.SOCK_RAW, unix.IPPROTO_UDP)
				if err != nil {
					return err
				}
				if err := unix.SetsockoptInt(bfd, unix.SOL_SOCKET, unix.SO_REUSEADDR, 1); err != nil {
					r.logger.Info("setsockopt reuseaddr on broadcast socket", "err", err)
				}
				if err := unix.SetsockoptInt(bfd, unix.SOL_SOCKET, unix.SO_BROADCAST, 1); err != nil {
					r.logger.Info("setsockopt broadcast", "err", err)
				}
				if err := unix.Bind(bfd, &unix.SockaddrInet4{Port: port}); err != nil {
					_ = unix.Close(bfd)
					return fmt.Errorf("bind broadcast socket: %w", err)
				}
				r.receivers = append(r.receivers, receiver{fd: bfd})
			} else if isMulticast(addr) {
				var mreq unix.IPMreqn
				copy(mreq.Multiaddr[:], parsedIP)
				ifaceIP := net.ParseIP(info.IP).To4()
				copy(mreq.Address[:], ifaceIP)
				if err := unix.SetsockoptIPMreqn(fd, unix.IPPROTO_IP, unix.IP_ADD_MEMBERSHIP, &mreq); err != nil {
					r.logger.Info("join multicast group", "addr", addr, "iface", info.Name, "err", err)
				}
			}

			if !containsStr(r.cfg.NoTransmitIfaces, info.Name) {
				txfd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_RAW, int(htons(etherTypeIPv4)))
				if err != nil {
					return fmt.Errorf("create packet socket for %s: %w", info.Name, err)
				}
				if err := bindToDevice(txfd, info.Name); err != nil {
					_ = unix.Close(txfd)
					return fmt.Errorf("bind packet socket to %s: %w", info.Name, err)
				}

				r.transmitters = append(r.transmitters, transmitter{
					relayAddr: addr,
					ifindex:   info.IfIndex,
					relayPort: port,
					iface:     info.Name,
					ip:        info.IP,
					mac:       info.MAC,
					netmask:   info.Netmask,
					broadcast: info.Broadcast,
					fd:        txfd,
					service:   service,
				})
			}
		}

		var sa unix.SockaddrInet4
		copy(sa.Addr[:], parsedIP)
		sa.Port = port
		if err := unix.Bind(fd, &sa); err != nil {
			_ = unix.Close(fd)
			return fmt.Errorf("bind mcast socket: %w", err)
		}
		r.receivers = append(r.receivers, receiver{fd: fd})
	} else {
		for _, ifaceSpec := range r.cfg.Interfaces {
			info, err := r.nif.Resolve(ifaceSpec)
			if err != nil {
				return err
			}

			if !containsStr(r.cfg.NoTransmitIfaces, info.Name) {
				txfd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_RAW, int(htons(etherTypeIPv4)))
				if err != nil {
					return fmt.Errorf("create packet socket for %s: %w", info.Name, err)
				}
				if err := bindToDevice(txfd, info.Name); err != nil {
					_ = unix.Close(txfd)
					return fmt.Errorf("bind packet socket to %s: %w", info.Name, err)
				}

				r.transmitters = append(r.transmitters, transmitter{
					relayAddr: addr,
					ifindex:   info.IfIndex,
					relayPort: port,
					iface:     info.Name,
					ip:        info.IP,
					mac:       info.MAC,
					netmask:   info.Netmask,
					broadcast: info.Broadcast,
					fd:        txfd,
					service:   service,
				})
			}
		}
	}

	for _, ifaceSpec := range r.cfg.Interfaces {
		info, err := r.nif.Resolve(ifaceSpec)
		if err != nil {
			continue
		}
		dup := false
		for _, a := range r.allIfaces {
			if a.iface == info.Name && a.relayAddr == addr {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		entry := transmitter{
			relayAddr: addr,
			ifindex:   info.IfIndex,
			relayPort: port,
			iface:     info.Name,
			ip:        info.IP,
			mac:       info.MAC,
			netmask:   info.Netmask,
			broadcast: info.Broadcast,
			service:   service,
		}
		if containsStr(r.cfg.NoTransmitIfaces, info.Name) {
			rxfd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_RAW, int(htons(etherTypeIPv4)))
			if err != nil {
				return fmt.Errorf("create rx packet socket for %s: %w", info.Name, err)
			}
			sll := &unix.SockaddrLinklayer{
				Protocol: htons(etherTypeIPv4),
				Ifindex:  info.IfIndex,
			}
			if err := unix.Bind(rxfd, sll); err != nil {
				_ = unix.Close(rxfd)
				return fmt.Errorf("bind rx packet socket to %s: %w", info.Name, err)
			}
			entry.fd = rxfd
			entry.rxOnly = true
			r.receivers = append(r.receivers, receiver{fd: rxfd, etherHdr: true})
		}
		r.allIfaces = append(r.allIfaces, entry)
	}

	r.bindings[[2]string{addr, fmt.Sprintf("%d", port)}] = true
	return nil
}

func bindToDevice(fd int, device string) error {
	return unix.SetsockoptString(fd, unix.SOL_SOCKET, unix.SO_BINDTODEVICE, device)
}

func (r *Relay) Run(ctx context.Context) error {
	if !r.cfg.Foreground {
		r.logger.Info("running in background (daemon mode not implemented in Go build)")
	}

	recentSSDP := make(map[string]uint16)
	bufPtr := bufPool.Get().(*[]byte)
	defer bufPool.Put(bufPtr)
	buf := *bufPtr

	pollFds := make([]unix.PollFd, 0, len(r.receivers))
	for _, rc := range r.receivers {
		pollFds = append(pollFds, unix.PollFd{Fd: int32(rc.fd), Events: unix.POLLIN})
	}

	for _, conn := range r.remoteConns {
		go r.readRemote(conn)
	}
	for i := range r.remoteAddrs {
		if r.remoteAddrs[i].conn != nil {
			go r.readRemoteOutbound(r.remoteAddrs[i].conn)
		}
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		if r.listenSock != nil {
			r.acceptRemote()
		}
		r.connectRemotes()

		if len(pollFds) == 0 && r.remoteCh == nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		if len(pollFds) > 0 {
			_, err := unix.Poll(pollFds, 100)
			if err != nil && err != unix.EINTR {
				r.logger.Info("poll error", "err", err)
			}

			for i := range pollFds {
				if pollFds[i].Revents&unix.POLLIN == 0 {
					continue
				}

				n, _, err := unix.Recvfrom(int(pollFds[i].Fd), buf, 0)
				if err != nil {
					if err == unix.EAGAIN || err == unix.EINTR {
						continue
					}
					r.logger.Info("recv error", "err", err)
					continue
				}

				offset := 0
				if r.receivers[i].etherHdr {
					if n < 34 {
						continue
					}
					offset = 14
				}
				if n-offset < 20 {
					continue
				}

				pktPtr := pktPool.Get().(*[]byte)
				data := (*pktPtr)[:n-offset]
				copy(data, buf[offset:n])
				r.handlePacket(data, "local", &recentSSDP)
				pktPool.Put(pktPtr)
			}
		}

		select {
		case pkt := <-r.remoteCh:
			r.handlePacket(pkt, "remote", &recentSSDP)
		default:
		}
	}
}

func (r *Relay) readRemote(conn *net.TCPConn) {
	sizeBuf := make([]byte, 2)
	for {
		if _, err := conn.Read(sizeBuf); err != nil {
			return
		}
		size := binary.BigEndian.Uint16(sizeBuf)
		pkt := make([]byte, size)
		if _, err := conn.Read(pkt); err != nil {
			return
		}
		if r.aes.enabled() {
			decrypted, err := r.aes.decrypt(pkt)
			if err != nil {
				return
			}
			pkt = decrypted
		}
		if len(pkt) < magicLen+4 || string(pkt[:magicLen]) != "MRLY" {
			return
		}
		r.remoteCh <- pkt
	}
}

func (r *Relay) readRemoteOutbound(conn *net.TCPConn) {
	sizeBuf := make([]byte, 2)
	for {
		if _, err := conn.Read(sizeBuf); err != nil {
			return
		}
		size := binary.BigEndian.Uint16(sizeBuf)
		pkt := make([]byte, size)
		if _, err := conn.Read(pkt); err != nil {
			return
		}
		if r.aes.enabled() {
			decrypted, err := r.aes.decrypt(pkt)
			if err != nil {
				return
			}
			pkt = decrypted
		}
		if len(pkt) < magicLen+4 || string(pkt[:magicLen]) != "MRLY" {
			return
		}
		r.remoteCh <- pkt
	}
}

func (r *Relay) handlePacket(data []byte, source string, recentSSDP *map[string]uint16) {
	if len(data) < 20 {
		return
	}

	ipHdrLen := int(data[0]&0x0f) * 4
	if ipHdrLen > len(data) || ipHdrLen < 20 {
		return
	}

	srcIP := data[12:16]
	dstIP := data[16:20]

	if ipHdrLen+4 > len(data) {
		return
	}
	srcPort := binary.BigEndian.Uint16(data[ipHdrLen : ipHdrLen+2])
	dstPort := binary.BigEndian.Uint16(data[ipHdrLen+2 : ipHdrLen+4])

	srcAddr := ip4Str(srcIP)
	dstAddr := ip4Str(dstIP)

	receivingIface := ""
	if source == "local" {
		for _, tx := range r.allIfaces {
			if tx.relayAddr == dstAddr && tx.relayPort == int(dstPort) && onNetworkIP(srcIP, tx.ip, tx.netmask) {
				receivingIface = tx.iface
				break
			}
		}
		if receivingIface == "" {
			return
		}
		if !r.bindings[[2]string{dstAddr, fmt.Sprintf("%d", dstPort)}] {
			return
		}
	} else {
		receivingIface = "remote"
	}

	ttl := data[8]
	if r.cfg.TTL != 0 {
		data[8] = byte(r.cfg.TTL)
	}

	ipChecksum := binary.BigEndian.Uint16(data[10:12])
	if slices.Contains(r.recentChecksum, ipChecksum) {
		return
	}
	r.recentChecksum[r.cksumIdx] = ipChecksum
	r.cksumIdx = (r.cksumIdx + 1) % len(r.recentChecksum)

	origSrcAddr := srcAddr
	origSrcPort := srcPort
	origDstAddr := dstAddr
	origDstPort := dstPort

	var destMAC net.HardwareAddr

	if r.cfg.MDNSForceUnicast && dstAddr == MDNSMcastAddr && dstPort == MDNSMcastPort {
		data = mdnsSetUnicastBit(data, ipHdrLen)
	}

	if r.cfg.SSDPUnicastAddr != "" && dstAddr == SSDPMcastAddr && dstPort == SSDPMcastPort {
		if bytesContains(data[ipHdrLen+8:], []byte("M-SEARCH")) || bytesContains(data[ipHdrLen+8:], []byte("NOTIFY")) {
			(*recentSSDP)[srcAddr] = srcPort
			srcAddr = r.cfg.SSDPUnicastAddr
			srcPort = SSDPUnicastPort
			data = modifyUDPPacket(data, ipHdrLen, net.ParseIP(srcAddr), nil, &srcPort, nil)
			dstAddr = ip4Str(data[16:20])
		}
	} else if r.cfg.SSDPUnicastAddr != "" && origDstAddr == r.cfg.SSDPUnicastAddr && origDstPort == SSDPUnicastPort {
		lastPort, ok := (*recentSSDP)[srcAddr]
		if !ok {
			return
		}
		mac, err := unicastIPToMAC(srcAddr)
		if err != nil || mac == nil {
			r.logger.Info("cannot resolve MAC", "ip", srcAddr, "err", err)
			return
		}
		destMAC = mac
		dstPort = lastPort
		data = modifyUDPPacket(data, ipHdrLen, nil, net.ParseIP(srcAddr), nil, &dstPort)
		dstAddr = srcAddr
	}

	if source == "local" {
		broadcastPkt := false
		for _, tx := range r.transmitters {
			if origDstAddr == tx.relayAddr && origDstPort == uint16(tx.relayPort) && onNetworkIP(data[12:16], tx.ip, tx.netmask) {
				receivingIface = tx.iface
				if origDstAddr == tx.broadcast {
					broadcastPkt = true
				}
			}
		}

		for _, tx := range r.transmitters {
			if receivingIface == tx.iface {
				continue
			}

			if r.cfg.FilterMap != nil {
				skip := false
				for network, ifaces := range r.cfg.FilterMap {
					parts := strings.SplitN(network, "/", 2)
					netStr := parts[0]
					maskBits := 32
					if len(parts) == 2 {
						maskBits = mustAtoi(parts[1])
					}
					if onNetwork(srcAddr, netStr, cidrToNetmask(maskBits)) {
						if !slices.Contains(ifaces, tx.iface) {
							skip = true
							break
						}
					}
				}
				if skip {
					continue
				}
			}

			pktBufPtr := pktPool.Get().(*[]byte)
			pktData := (*pktBufPtr)[:len(data)]
			copy(pktData, data)

			curDstAddr := dstAddr
			curDstPort := dstPort
			curSrcAddr := srcAddr
			curSrcPort := srcPort
			mac := destMAC
			if mac == nil {
				if m, ok := r.etherAddrs[curDstAddr]; ok {
					mac = m
				}
			}

			if broadcastPkt {
				curDstAddr = tx.broadcast
				mac = broadcastIPToMAC()
				origDstAddr = tx.broadcast
				copy(pktData[16:20], net.ParseIP(tx.broadcast).To4())
			}

			if origDstAddr == tx.relayAddr && origDstPort == uint16(tx.relayPort) &&
				(r.cfg.OneInterface || !onNetworkIP(pktData[12:16], tx.ip, tx.netmask)) {

				if containsStr(r.cfg.Masquerade, tx.iface) {
					copy(pktData[12:16], net.ParseIP(tx.ip).To4())
					curSrcAddr = tx.ip
				}

				if r.cfg.Verbose {
					service := ""
					if tx.service != "" {
						service = "[" + tx.service + "] "
					}
					masqStr := "Relayed"
					if containsStr(r.cfg.Masquerade, tx.iface) {
						masqStr = "Masqueraded"
					}
					asSrc := ""
					if curSrcAddr != origSrcAddr || curSrcPort != origSrcPort {
						asSrc = fmt.Sprintf(" (as %s:%d)", curSrcAddr, curSrcPort)
					}
					r.logger.Info(fmt.Sprintf("%s%s %d bytes from %s:%d on %s [ttl %d] to %s:%d via %s/%s%s",
						service, masqStr, len(pktData), origSrcAddr, origSrcPort, receivingIface, ttl, curDstAddr, curDstPort, tx.iface, tx.ip, asSrc))
				}

				r.transmitPacket(tx, mac, ipHdrLen, pktData)
				pktPool.Put(pktBufPtr)
			}
		}
	}
}

func (r *Relay) transmitPacket(tx transmitter, destMAC net.HardwareAddr, ipHdrLen int, ipPacket []byte) {
	ipHeader := ipPacket[:ipHdrLen]
	udpHeader := ipPacket[ipHdrLen : ipHdrLen+8]
	payload := ipPacket[ipHdrLen+8:]

	srcPort := binary.BigEndian.Uint16(udpHeader[0:2])
	dstPort := binary.BigEndian.Uint16(udpHeader[2:4])
	udpChecksum := computeUDPChecksum(ipHeader, payload, srcPort, dstPort)

	dontFrag := (ipPacket[6] & 0x40) >> 6

	for boundary := 0; boundary < len(payload); boundary += udpMaxLength {
		end := min(boundary+udpMaxLength, len(payload))
		dataFrag := payload[boundary:end]
		totalLen := ipHdrLen + 8 + len(dataFrag)
		moreFrags := boundary+udpMaxLength < len(payload)

		flagsOffset := uint16(boundary & 0x1fff)
		if moreFrags {
			flagsOffset |= 0x2000
		} else if dontFrag != 0 {
			flagsOffset |= 0x4000
		}

		txBufPtr := txBufPool.Get().(*[]byte)
		txBuf := *txBufPtr
		outIP := txBuf[:totalLen]
		copy(outIP, ipHeader)
		binary.BigEndian.PutUint16(outIP[2:4], uint16(totalLen))
		binary.BigEndian.PutUint16(outIP[6:8], flagsOffset)
		copy(outIP[ipHdrLen:], udpHeader[:6])
		binary.BigEndian.PutUint16(outIP[ipHdrLen+6:ipHdrLen+8], udpChecksum)
		copy(outIP[ipHdrLen+8:], dataFrag)

		outIP[10] = 0
		outIP[11] = 0
		chksum := computeIPChecksum(outIP[:ipHdrLen])
		binary.BigEndian.PutUint16(outIP[10:12], chksum)

		if len(destMAC) == 6 && (destMAC[0]|destMAC[1]|destMAC[2]|destMAC[3]|destMAC[4]|destMAC[5]) != 0 {
			etherFrame := txBuf[totalLen : totalLen+14+len(outIP)]
			copy(etherFrame[0:6], destMAC)
			copy(etherFrame[6:12], tx.mac)
			binary.BigEndian.PutUint16(etherFrame[12:14], etherTypeIPv4)
			copy(etherFrame[14:], outIP)

			var hwAddr [8]byte
			copy(hwAddr[:6], destMAC)
			sa := &unix.SockaddrLinklayer{
				Protocol: htons(etherTypeIPv4),
				Ifindex:  tx.ifindex,
				Hatype:   unix.ARPHRD_ETHER,
				Pkttype:  0,
				Halen:    6,
				Addr:     hwAddr,
			}
			if err := unix.Sendto(tx.fd, etherFrame, 0, sa); err != nil {
				r.logger.Info("send error", "iface", tx.iface, "err", err)
			}
		} else {
			var addr unix.SockaddrInet4
			copy(addr.Addr[:], outIP[16:20])
			if err := unix.Sendto(tx.fd, outIP, 0, &addr); err != nil {
				r.logger.Info("send error", "iface", tx.iface, "err", err)
			}
		}
		txBufPool.Put(txBufPtr)
	}
}

func (r *Relay) connectRemotes() {
	for i := range r.remoteAddrs {
		rc := &r.remoteAddrs[i]
		if rc.conn != nil {
			continue
		}
		if !rc.failAt.IsZero() && time.Since(rc.failAt) < time.Duration(r.cfg.RemoteRetry)*time.Second {
			continue
		}
		r.logger.Info("REMOTE: connecting", "addr", rc.addr)
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", rc.addr, r.cfg.RemotePort), 5*time.Second)
		if err != nil {
			r.logger.Info("REMOTE: connect failed", "addr", rc.addr, "err", err)
			rc.failAt = time.Now()
			continue
		}
		rc.conn = conn.(*net.TCPConn)
		rc.failAt = time.Time{}
		r.logger.Info("REMOTE: connected", "addr", rc.addr)
	}
}

func (r *Relay) acceptRemote() {
	if r.listenSock == nil {
		return
	}
	conn, err := r.listenSock.Accept()
	if err != nil {
		return
	}
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		_ = conn.Close()
		return
	}
	addr := conn.RemoteAddr().(*net.TCPAddr).IP.String()
	if !slices.Contains(r.cfg.ListenAddrs, addr) {
		r.logger.Info("refusing connection", "addr", addr)
		_ = conn.Close()
		return
	}
	r.remoteConns = append(r.remoteConns, tcpConn)
	r.logger.Info("REMOTE: accepted connection", "addr", addr)
}

func htons(v uint16) uint16 {
	return (v<<8)&0xff00 | v>>8
}

func mustAtoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

func bytesContains(data, pattern []byte) bool {
	for i := 0; i <= len(data)-len(pattern); i++ {
		if bytesEqual(data[i:i+len(pattern)], pattern) {
			return true
		}
	}
	return false
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
