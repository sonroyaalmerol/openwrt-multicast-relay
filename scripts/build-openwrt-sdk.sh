#!/bin/bash
set -euo pipefail

OPENWRT_VERSION="${OPENWRT_VERSION:-25.12.4}"
TARGET="${1:-ipq806x}"
JOBS="${JOBS:-$(nproc)}"
WORKDIR="$(cd "$(dirname "$0")/.." && pwd)"
SDK_DIR="${WORKDIR}/sdk"

declare -A TARGET_MAP=(
	[ipq806x]="ipq806x/generic eabi"
)

TARGET_PATH="${TARGET_MAP[$TARGET]%% *}"
SUBTARGET="${TARGET_PATH#*/}"
BOARD="${TARGET_PATH%%/*}"
ARCH_SUFFIX="${TARGET_MAP[$TARGET]##* }"

SDK_URL="https://downloads.openwrt.org/releases/${OPENWRT_VERSION}/targets/${TARGET_PATH}/openwrt-sdk-${OPENWRT_VERSION}-${BOARD}-${SUBTARGET}_gcc-14.3.0_musl_${ARCH_SUFFIX}.Linux-x86_64.tar.zst"
SDK_SHA256_URL="https://downloads.openwrt.org/releases/${OPENWRT_VERSION}/targets/${TARGET_PATH}/sha256sums"

echo "=== OpenWrt SDK Build ==="
echo "Version: ${OPENWRT_VERSION}"
echo "Target:  ${TARGET} (${TARGET_PATH})"

if [ ! -d "${SDK_DIR}/sdk" ]; then
	echo ">>> Downloading SDK..."
	mkdir -p "${SDK_DIR}"
	SDK_TAR="${SDK_DIR}/sdk.tar.zst"

	if [ ! -f "${SDK_TAR}" ]; then
		curl -fSL -o "${SDK_TAR}" "${SDK_URL}"
	fi

	echo ">>> Verifying SDK checksum..."
	SHA256_EXPECTED=$(curl -fSL "${SDK_SHA256_URL}" | grep "$(basename "${SDK_URL}")" | awk '{print $1}')
	SHA256_ACTUAL=$(sha256sum "${SDK_TAR}" | awk '{print $1}')
	if [ "${SHA256_EXPECTED}" != "${SHA256_ACTUAL}" ]; then
		echo "ERROR: SDK checksum mismatch"
		echo "Expected: ${SHA256_EXPECTED}"
		echo "Actual:   ${SHA256_ACTUAL}"
		exit 1
	fi

	echo ">>> Extracting SDK..."
	tar --zstd -xf "${SDK_TAR}" -C "${SDK_DIR}"
	SDK_EXTRACTED=$(find "${SDK_DIR}" -maxdepth 1 -name "openwrt-sdk-*" -type d | head -1)
	mv "${SDK_EXTRACTED}" "${SDK_DIR}/sdk"
fi

SDK="${SDK_DIR}/sdk"
echo "SDK path: ${SDK}"

echo ">>> Setting up feeds..."
cp "${SDK}/feeds.conf.default" "${SDK}/feeds.conf"
echo "src-link multicast_relay ${WORKDIR}/openwrt" >> "${SDK}/feeds.conf"

export PATH="${HOME}/bin:${PATH}"
if ! command -v unzip &>/dev/null; then
	mkdir -p "${HOME}/bin"
	cat > "${HOME}/bin/unzip" << 'WRAPPER'
#!/bin/sh
exec python3 -c "
import zipfile, sys
zf = zipfile.ZipFile(sys.argv[1])
if len(sys.argv) > 2:
    zf.extract(sys.argv[2])
else:
    zf.extractall()
"
WRAPPER
	chmod +x "${HOME}/bin/unzip"
fi
if ! command -v wget &>/dev/null || ! wget --version 2>&1 | grep -q GNU; then
	mkdir -p "${HOME}/bin"
	cat > "${HOME}/bin/wget" << 'WRAPPER'
#!/bin/sh
if [ "$1" = "--version" ]; then
    echo "GNU Wget 1.25.0"
    exit 0
fi
exec curl -L "$@"
WRAPPER
	chmod +x "${HOME}/bin/wget"
fi

echo ">>> Patching GOTOOLCHAIN=local into SDK golang infrastructure..."
python3 "${WORKDIR}/scripts/patch-sdk-gotochain.py" "${SDK}/feeds/packages/lang/golang"

echo ">>> Updating feeds..."
(cd "${SDK}" && ./scripts/feeds update -a 2>&1 | tail -10)

echo ">>> Installing feeds..."
(cd "${SDK}" && ./scripts/feeds install -a 2>&1 | tail -10)

echo ">>> Configuring packages..."
(cd "${SDK}" && {
	echo "CONFIG_PACKAGE_multicast-relay=y"
	echo "CONFIG_PACKAGE_luci-app-multicast-relay=y"
} >> .config && make defconfig 2>&1 | tail -5)

echo ">>> Building multicast-relay..."
(cd "${SDK}" && make package/multicast-relay/compile V=s -j${JOBS} 2>&1 | tail -30)

echo ">>> Building luci-app-multicast-relay..."
(cd "${SDK}" && make package/luci-app-multicast-relay/compile V=s -j${JOBS} 2>&1 | tail -30)

echo ">>> Collecting packages..."
OUTDIR="${WORKDIR}/dist"
mkdir -p "${OUTDIR}"
find "${SDK}/bin" -name "multicast-relay-*.apk" -exec cp -v {} "${OUTDIR}/" \;
find "${SDK}/bin" -name "luci-app-multicast-relay-*.apk" -exec cp -v {} "${OUTDIR}/" \;

echo "=== Done ==="
ls -la "${OUTDIR}"/*.apk 2>/dev/null || echo "No APK packages found"