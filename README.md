# openwrt-multicast-relay

Multicast relay for OpenWrt. Relay mDNS, SSDP, and other multicast/broadcast traffic between network interfaces.

Lightweight Go binary (~2 MB) packaged as an OpenWrt APK. No runtime dependencies.

## Features

- Relay mDNS (224.0.0.251:5353) between interfaces
- Relay SSDP (239.255.255.250:1900) between interfaces
- Relay Sonos discovery broadcasts
- SSDP unicast proxy mode
- mDNS force-unicast-response bit
- Masquerade mode (NAT-like source IP rewriting)
- Remote relay over TCP with optional AES encryption
- Interface filtering (JSON config)
- OpenWrt procd init script included

## Installation

Download the APK package from [releases](https://github.com/sonroyaalmerol/openwrt-multicast-relay/releases) and install:

```sh
opkg install multicast-relay_*.apk
```

Or build from source:

```sh
go build ./cmd/multicast-relay
```

## Usage

### Command Line

```sh
multicast-relay --foreground --interfaces 192.168.1.0/24,192.168.3.0/24
```

### OpenWrt UCI Configuration

```sh
uci set multicast-relay.general.enabled=1
uci set multicast-relay.general.interfaces='192.168.1.0/24 192.168.3.0/24'
uci commit multicast-relay
/etc/init.d/multicast-relay start
```

### Options

| Flag | Description |
|------|-------------|
| `--interfaces` | Comma-separated list of interfaces/networks to relay between (required, min 2) |
| `--noTransmitInterfaces` | Interfaces to listen on but not transmit from |
| `--ssdpUnicastAddr` | IP address for SSDP unicast reply relay |
| `--oneInterface` | Single interface connected to two networks |
| `--relay` | Additional multicast/broadcast address:port to relay |
| `--noMDNS` | Do not relay mDNS |
| `--noSSDP` | Do not relay SSDP |
| `--noSonosDiscovery` | Do not relay Sonos discovery |
| `--mdnsForceUnicast` | Force mDNS UNICAST-RESPONSE bit |
| `--allowNonEther` | Allow non-ethernet interfaces |
| `--masquerade` | Masquerade packets from specified interfaces |
| `--ttl` | Set TTL on outbound packets |
| `--wait` | Wait for IPv4 address assignment |
| `--listen` | Listen for remote relay connections (comma-separated IPs) |
| `--remote` | Connect to remote relay (comma-separated IPs) |
| `--remotePort` | TCP port for remote relay (default: 1900) |
| `--aes` | AES encryption key for remote relay |
| `--foreground` | Run in foreground |
| `--logfile` | Log file path |
| `--verbose` | Enable verbose logging |

## Building for OpenWrt

```sh
goreleaser build --snapshot --clean
```

Cross-compilation targets:
- `armhf` (armv7, GOARM=7): most OpenWrt routers
- `arm64`: newer routers
- `amd64`: x86 routers/VMs

## License

MIT