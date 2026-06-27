
include $(TOPDIR)/rules.mk

PKG_NAME:=multicast-relay
PKG_VERSION:=0.1.4
PKG_RELEASE:=1

PKG_SOURCE_PROTO:=git
PKG_SOURCE_URL:=https://github.com/sonroyaalmerol/openwrt-multicast-relay
PKG_SOURCE_VERSION:=v$(PKG_VERSION)
PKG_MIRROR_HASH:=skip

PKG_MAINTAINER:=sonroyaalmerol
PKG_LICENSE:=MIT
PKG_BUILD_DEPENDS:=golang/host
PKG_BUILD_PARALLEL:=1

GO_PKG:=github.com/sonroyaalmerol/openwrt-multicast-relay/cmd/multicast-relay
GO_PKG_LDFLAGS:=-s -w
GO_PKG_LDFLAGS_X:=main.version=$(PKG_VERSION)

include $(INCLUDE_DIR)/package.mk
include $(TOPDIR)/feeds/packages/lang/golang/golang-package.mk

define Package/multicast-relay
	SECTION:=net
	CATEGORY:=Network
	TITLE:=Multicast relay for mDNS/SSDP
	URL:=https://github.com/sonroyaalmerol/openwrt-multicast-relay
	DEPENDS:=$(GO_ARCH_DEPENDS)
endef

define Package/multicast-relay/description
	Relay mDNS, SSDP, and other multicast/broadcast traffic
	between network interfaces.
endef

define Package/multicast-relay/conffiles
/etc/config/multicast-relay
endef

define Package/multicast-relay/install
	$(call GoPackage/Package/Install/Bin,$(1))

	$(INSTALL_DIR) $(1)/etc/init.d
	$(INSTALL_BIN) ./files/etc/init.d/multicast-relay $(1)/etc/init.d/multicast-relay

	$(INSTALL_DIR) $(1)/etc/config
	$(INSTALL_CONF) ./files/etc/config/multicast-relay $(1)/etc/config/multicast-relay
endef

define Package/multicast-relay/postinst
[ -n "$${IPKG_INSTROOT}" ] || /etc/init.d/multicast-relay enable
endef

define Package/multicast-relay/postrm
[ -n "$${IPKG_INSTROOT}" ] || /etc/init.d/multicast-relay disable
endef

$(eval $(call GoBinPackage,multicast-relay))
$(eval $(call BuildPackage,multicast-relay))