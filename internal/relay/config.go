package relay

import (
	"flag"
	"fmt"
	"net"
	"slices"
	"strings"
)

type Config struct {
	Interfaces       []string
	NoTransmitIfaces []string
	FilterFile       string
	SSDPUnicastAddr  string
	OneInterface     bool
	RelayAddrs       []string
	NoMDNS           bool
	MDNSForceUnicast bool
	NoSSDP           bool
	NoSonosDiscovery bool
	AllowNonEther    bool
	Masquerade       []string
	WaitForIP        bool
	TTL              int
	ListenAddrs      []string
	RemoteAddrs      []string
	RemotePort       int
	RemoteRetry      int
	NoRemoteRelay    bool
	AESKey           string
	Foreground       bool
	Logfile          string
	Verbose          bool
	FilterMap        map[string][]string
}

func ParseConfig(args []string) (*Config, error) {
	fs := flag.NewFlagSet("multicast-relay", flag.ContinueOnError)

	ifaces := fs.String("interfaces", "", "Relay between these interfaces (comma-separated, min 2)")
	noTransmit := fs.String("noTransmitInterfaces", "", "Do not relay via these interfaces (comma-separated)")
	ifFilter := fs.String("ifFilter", "", "JSON filter file")
	ssdpUnicast := fs.String("ssdpUnicastAddr", "", "IP for SSDP unicast replies")
	oneIface := fs.Bool("oneInterface", false, "Only one interface connected to two networks")
	relay := fs.String("relay", "", "Additional multicast addresses (comma-separated)")
	noMDNS := fs.Bool("noMDNS", false, "Do not relay mDNS packets")
	mdnsForceUnicast := fs.Bool("mdnsForceUnicast", false, "Force mDNS UNICAST-RESPONSE bit")
	noSSDP := fs.Bool("noSSDP", false, "Do not relay SSDP packets")
	noSonos := fs.Bool("noSonosDiscovery", false, "Do not relay Sonos discovery")
	allowNonEther := fs.Bool("allowNonEther", false, "Allow non-ethernet interfaces")
	masquerade := fs.String("masquerade", "", "Masquerade from these interfaces (comma-separated)")
	waitForIP := fs.Bool("wait", false, "Wait for IPv4 address assignment")
	ttl := fs.Int("ttl", 0, "Set TTL on outbound packets (0 = don't modify)")
	listen := fs.String("listen", "", "Listen for remote connection (comma-separated addrs)")
	remote := fs.String("remote", "", "Relay to remote addresses (comma-separated)")
	remotePort := fs.Int("remotePort", 1900, "Remote connection port")
	remoteRetry := fs.Int("remoteRetry", 5, "Remote retry interval in seconds")
	noRemoteRelay := fs.Bool("noRemoteRelay", false, "Don't relay packets from remote connections")
	aes := fs.String("aes", "", "AES encryption key for remote relay")
	foreground := fs.Bool("foreground", false, "Run in foreground")
	logfile := fs.String("logfile", "", "Log file path")
	verbose := fs.Bool("verbose", false, "Enable verbose output")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	cfg := &Config{
		Interfaces:       splitInterfaces(*ifaces),
		NoTransmitIfaces: splitNonEmpty(*noTransmit),
		FilterFile:       *ifFilter,
		SSDPUnicastAddr:  *ssdpUnicast,
		OneInterface:     *oneIface,
		RelayAddrs:       splitNonEmpty(*relay),
		NoMDNS:           *noMDNS,
		MDNSForceUnicast: *mdnsForceUnicast,
		NoSSDP:           *noSSDP,
		NoSonosDiscovery: *noSonos,
		AllowNonEther:    *allowNonEther,
		Masquerade:       splitNonEmpty(*masquerade),
		WaitForIP:        *waitForIP,
		TTL:              *ttl,
		ListenAddrs:      splitNonEmpty(*listen),
		RemoteAddrs:      splitNonEmpty(*remote),
		RemotePort:       *remotePort,
		RemoteRetry:      *remoteRetry,
		NoRemoteRelay:    *noRemoteRelay,
		AESKey:           *aes,
		Foreground:       *foreground,
		Logfile:          *logfile,
		Verbose:          *verbose,
	}

	if len(cfg.Interfaces) < 2 && !cfg.OneInterface && len(cfg.ListenAddrs) == 0 && len(cfg.RemoteAddrs) == 0 {
		return nil, fmt.Errorf("specify at least two interfaces to relay between")
	}
	if len(cfg.RemoteAddrs) > 0 && len(cfg.ListenAddrs) > 0 {
		return nil, fmt.Errorf("relay role should be either --listen or --remote, not both")
	}
	if cfg.TTL != 0 && (cfg.TTL < 1 || cfg.TTL > 255) {
		return nil, fmt.Errorf("invalid TTL (must be between 1 and 255)")
	}

	return cfg, nil
}

func splitNonEmpty(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	for v := range strings.SplitSeq(s, ",") {
		v = strings.TrimSpace(v)
		if v != "" {
			result = append(result, v)
		}
	}
	return result
}

func splitInterfaces(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	for field := range strings.FieldsSeq(s) {
		if strings.Contains(field, ",") {
			for v := range strings.SplitSeq(field, ",") {
				v = strings.TrimSpace(v)
				if v != "" {
					result = append(result, v)
				}
			}
		} else {
			result = append(result, field)
		}
	}
	return result
}

type relaySpec struct {
	Addr    string
	Port    int
	Service string
}

func (c *Config) DefaultRelays() []relaySpec {
	var relays []relaySpec
	if !c.NoMDNS {
		relays = append(relays, relaySpec{MDNSMcastAddr, MDNSMcastPort, "mDNS"})
	}
	if !c.NoSSDP {
		relays = append(relays, relaySpec{SSDPMcastAddr, SSDPMcastPort, "SSDP"})
	}
	if !c.NoSonosDiscovery {
		relays = append(relays, relaySpec{Broadcast, 6969, "Sonos Setup Discovery"})
	}
	if c.SSDPUnicastAddr != "" {
		relays = append(relays, relaySpec{c.SSDPUnicastAddr, SSDPUnicastPort, "SSDP Unicast"})
	}
	for _, r := range c.RelayAddrs {
		relays = append(relays, relaySpec{r, 0, ""})
	}
	return relays
}

func (c *Config) ValidateRelays(relays []relaySpec) error {
	for _, r := range relays {
		if net.ParseIP(r.Addr) == nil {
			return fmt.Errorf("%s: invalid IP address", r.Addr)
		}
		if r.Port < 0 || r.Port > 65535 {
			return fmt.Errorf("port %d out of range for %s", r.Port, r.Addr)
		}
		if !isMulticast(r.Addr) && !isBroadcast(r.Addr) && c.SSDPUnicastAddr == "" {
			return fmt.Errorf("%s is neither multicast nor broadcast", r.Addr)
		}
	}
	return nil
}

func containsStr(ss []string, v string) bool {
	return slices.Contains(ss, v)
}
