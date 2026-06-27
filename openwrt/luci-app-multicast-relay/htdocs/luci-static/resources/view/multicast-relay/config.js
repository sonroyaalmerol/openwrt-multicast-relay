'use strict';
'require view';
'require uci';
'require form';
'require rpc';

var callInitAction = rpc.declare({
	object: 'luci',
	method: 'setInitAction',
	params: ['name', 'action'],
	expect: { result: false }
});

var callServiceList = rpc.declare({
	object: 'service',
	method: 'list',
	params: ['name'],
	expect: { }
});

return view.extend({
	load: function() {
		return uci.load('multicast-relay').then(function() {
		return callServiceList('multicast-relay');
	});
	},

	handleRestart: function(m, ev) {
		return callInitAction('multicast-relay', 'restart')
			.then(L.bind(m.render, m));
	},

	handleStop: function(m, ev) {
		return callInitAction('multicast-relay', 'stop')
			.then(L.bind(m.render, m));
	},

	handleStart: function(m, ev) {
		return callInitAction('multicast-relay', 'start')
			.then(L.bind(m.render, m));
	},

	render: function(data) {
		var running = !!(data && data['multicast-relay'] && data['multicast-relay'].running);

		var m, s, o;

		m = new form.Map('multicast-relay',
			_('Multicast Relay'),
			_('Relay mDNS, SSDP, and other multicast/broadcast traffic between network interfaces.'));

		s = m.section(form.NamedSection, 'main', 'multicast-relay', _('Service'));

		o = s.option(form.Button, '_restart', '&#160;');
		o.inputtitle = _('Restart');
		o.inputstyle = 'apply';
		o.onclick = L.bind(this.handleRestart, this, m);

		o = s.option(form.Button, '_stop', '&#160;');
		o.inputtitle = running ? _('Stop') : _('Start');
		o.inputstyle = running ? 'reset' : 'apply';
		o.onclick = running
			? L.bind(this.handleStop, this, m)
			: L.bind(this.handleStart, this, m);

		o = s.option(form.Flag, 'enabled', _('Enable'),
			_('Enable the multicast relay service.'));
		o.rmempty = false;
		o.default = '0';

		o = s.option(form.DynamicList, 'interfaces', _('Interfaces'),
			_('Network interfaces or subnets to relay between (e.g. 192.168.1.0/24, br-lan). Minimum 2 required.'));
		o.datatype = 'string';
		o.rmempty = false;

		o = s.option(form.Flag, 'foreground', _('Foreground'),
			_('Run in foreground (required for procd supervision).'));
		o.rmempty = false;
		o.default = '1';

		o = s.option(form.Flag, 'verbose', _('Verbose logging'),
			_('Enable verbose packet logging.'));
		o.rmempty = false;
		o.default = '0';

		o = s.option(form.Value, 'ttl', _('TTL'),
			_('Set TTL on outbound packets. 0 means do not modify.'));
		o.datatype = 'uinteger';
		o.optional = true;
		o.placeholder = '0';

		o = s.option(form.Value, 'ssdpUnicastAddr', _('SSDP Unicast Address'),
			_('Relay SSDP unicast replies to this IP address.'));
		o.datatype = 'ip4addr';
		o.optional = true;

		o = s.option(form.Flag, 'mdnsForceUnicast', _('Force mDNS Unicast'),
			_('Force the UNICAST-RESPONSE bit in mDNS answers.'));
		o.rmempty = false;
		o.default = '0';

		o = s.option(form.Flag, 'noMDNS', _('Disable mDNS'),
			_('Do not relay mDNS (224.0.0.251:5353) packets.'));
		o.rmempty = false;
		o.default = '0';

		o = s.option(form.Flag, 'noSSDP', _('Disable SSDP'),
			_('Do not relay SSDP (239.255.255.250:1900) packets.'));
		o.rmempty = false;
		o.default = '0';

		o = s.option(form.Flag, 'noSonosDiscovery', _('Disable Sonos Discovery'),
			_('Do not relay Sonos discovery broadcasts.'));
		o.rmempty = false;
		o.default = '0';

		o = s.option(form.Flag, 'allowNonEther', _('Allow Non-Ethernet'),
			_('Allow relay on non-ethernet interfaces.'));
		o.rmempty = false;
		o.default = '0';

		o = s.option(form.Flag, 'oneInterface', _('Single Interface'),
			_('Use when a single interface is connected to two networks.'));
		o.rmempty = false;
		o.default = '0';

		o = s.option(form.DynamicList, 'masquerade', _('Masquerade Interfaces'),
			_('Masquerade (NAT) packets from these interfaces.'));
		o.datatype = 'string';
		o.optional = true;

		o = s.option(form.DynamicList, 'noTransmitInterfaces', _('No-Transmit Interfaces'),
			_('Listen on but do not transmit from these interfaces.'));
		o.datatype = 'string';
		o.optional = true;

		o = s.option(form.DynamicList, 'relay', _('Additional Relays'),
			_('Additional multicast/broadcast address:port to relay (e.g. 239.255.255.250:1900).'));
		o.datatype = 'string';
		o.optional = true;

		o = s.option(form.Value, 'ifFilter', _('Interface Filter'),
			_('JSON filter file for interface-based packet filtering.'));
		o.datatype = 'file';
		o.optional = true;

		s = m.section(form.NamedSection, 'main', 'multicast-relay', _('Remote Relay'));

		o = s.option(form.DynamicList, 'listen', _('Listen Addresses'),
			_('Listen for remote relay connections (comma-separated IPs).'));
		o.datatype = 'ip4addr';
		o.optional = true;

		o = s.option(form.DynamicList, 'remote', _('Remote Addresses'),
			_('Connect to remote relay (comma-separated IPs).'));
		o.datatype = 'ip4addr';
		o.optional = true;

		o = s.option(form.Value, 'remotePort', _('Remote Port'),
			_('TCP port for remote relay connections.'));
		o.datatype = 'port';
		o.default = '1900';
		o.optional = true;

		o = s.option(form.Value, 'remoteRetry', _('Remote Retry Interval'),
			_('Seconds between remote relay reconnection attempts.'));
		o.datatype = 'uinteger';
		o.default = '5';
		o.optional = true;

		o = s.option(form.Value, 'aes', _('AES Key'),
			_('AES encryption key for remote relay traffic.'));
		o.password = true;
		o.optional = true;

		return m.render();
	}
});