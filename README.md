# openwrt-multicast-relay

Multicast relay for OpenWrt. Relay mDNS, SSDP, and other multicast/broadcast
traffic between network interfaces.

Lightweight Go binary with no runtime dependencies.

## Features

- Relay mDNS (224.0.0.251:5353) between interfaces
- Relay SSDP (239.255.255.250:1900) between interfaces
- Relay Sonos discovery broadcasts
- SSDP unicast proxy mode
- mDNS force-unicast-response bit
- Masquerade mode (NAT-like source IP rewriting)
- Remote relay over TCP with optional AES encryption
- Interface filtering (JSON config)
- LuCI web interface included

## Installation

### From feed

```sh
echo "src/gz openwrt-multicast-relay https://sonroyaalmerol.github.io/openwrt-multicast-relay" >> /etc/apk/repositories
apk add multicast-relay
apk add luci-app-multicast-relay
```

### From release

Download the APK for your architecture from
[releases](https://github.com/sonroyaalmerol/openwrt-multicast-relay/releases)
and install:

```sh
apk add --allow-untrusted multicast-relay-*_arm_cortex-a15_neon-vfpv4.apk
```

### From source

```sh
go build ./cmd/multicast-relay
```

## Usage

### Command line

```sh
multicast-relay --interfaces 192.168.1.0/24,192.168.3.0/24
```

### UCI configuration

```sh
uci set multicast-relay.general.enabled=1
uci set multicast-relay.general.interfaces='192.168.1.0/24 192.168.3.0/24'
uci commit multicast-relay
/etc/init.d/multicast-relay start
```

### Options

| Flag | Description |
|---|---|
| `--interfaces` | Comma-separated network prefixes to relay between (required, min 2) |
| `--noTransmitInterfaces` | Listen but don't retransmit from these |
| `--ssdpUnicastAddr` | IP for SSDP unicast reply relay |
| `--oneInterface` | Single interface connected to two networks |
| `--relay` | Additional multicast/broadcast address:port |
| `--noMDNS` | Do not relay mDNS |
| `--noSSDP` | Do not relay SSDP |
| `--noSonosDiscovery` | Do not relay Sonos discovery |
| `--mdnsForceUnicast` | Force mDNS UNICAST-RESPONSE bit |
| `--allowNonEther` | Allow non-ethernet interfaces |
| `--masquerade` | Masquerade packets from specified interfaces |
| `--ttl` | Set TTL on outbound packets |
| `--wait` | Wait for IPv4 address assignment |
| `--listen` | Listen for remote relay connections |
| `--remote` | Connect to remote relay |
| `--remotePort` | TCP port for remote relay (default: 1900) |
| `--aes` | AES encryption key for remote relay |
| `--foreground` | Run in foreground |
| `--logfile` | Log file path |
| `--verbose` | Verbose logging |

## Supported architectures

Prebuilt APKs are available for all 28 distinct CPU targets supported by
Go on OpenWrt 25.12:

aarch64_cortex-a53, aarch64_cortex-a55, aarch64_cortex-a72,
aarch64_cortex-a76, aarch64_generic, arm_cortex-a15_neon-vfpv4,
arm_cortex-a7_neon-vfpv4, arm_cortex-a9_neon,
arm_cortex-a9_vfpv3-d16, arm_arm1176jzf-s_vfp, arm_arm926ej-s,
arm_cortex-a5_vfpv4, arm_cortex-a7_vfpv4, arm_cortex-a8_vfpv3,
arm_xscale, i386_geode, i386_pentium-mmx, i386_pentium4,
loongarch64_generic, mips64_24kc, mips64el_24kc, mips_24kc,
mips_4kec, mips_mips32, mipsel_24kc, mipsel_mips32,
riscv64_rv64gc, x86_64

## License

MIT
