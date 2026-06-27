
include $(TOPDIR)/rules.mk

PKG_NAME:=luci-app-multicast-relay
PKG_VERSION:=0.1.4
PKG_RELEASE:=1

PKG_MAINTAINER:=sonroyaalmerol
PKG_LICENSE:=MIT

include $(INCLUDE_DIR)/package.mk

define Package/luci-app-multicast-relay
	SECTION:=luci
	CATEGORY:=LuCI
	SUBMENU:=3. Applications
	TITLE:=LuCI support for Multicast Relay
	DEPENDS:=+multicast-relay +luci-base
	PKGARCH:=all
endef

define Package/luci-app-multicast-relay/description
	LuCI web interface for configuring multicast-relay.
endef

define Package/luci-app-multicast-relay/install
	$(INSTALL_DIR) $(1)/usr/share/luci/menu.d
	$(INSTALL_DATA) ./root/usr/share/luci/menu.d/luci-app-multicast-relay.json $(1)/usr/share/luci/menu.d/

	$(INSTALL_DIR) $(1)/usr/share/rpcd/acl.d
	$(INSTALL_DATA) ./root/usr/share/rpcd/acl.d/luci-app-multicast-relay.json $(1)/usr/share/rpcd/acl.d/

	$(INSTALL_DIR) $(1)/www/luci-static/resources/view/multicast-relay
	$(INSTALL_DATA) ./htdocs/luci-static/resources/view/multicast-relay/config.js $(1)/www/luci-static/resources/view/multicast-relay/
endef

$(eval $(call BuildPackage,luci-app-multicast-relay))