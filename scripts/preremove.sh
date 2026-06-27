#!/bin/sh
/etc/init.d/multicast-relay stop 2>/dev/null || true
/etc/init.d/multicast-relay disable 2>/dev/null || true