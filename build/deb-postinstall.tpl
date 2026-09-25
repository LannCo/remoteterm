#!/bin/bash

if type update-alternatives 2>/dev/null >&1; then
    # Remove previous link if it doesn't use update-alternatives
    if [ -L '/usr/bin/remoteterm' -a -e '/usr/bin/remoteterm' -a "`readlink '/usr/bin/remoteterm'`" != '/etc/alternatives/remoteterm' ]; then
        rm -f '/usr/bin/remoteterm'
    fi
    update-alternatives --install '/usr/bin/remoteterm' 'remoteterm' '/opt/RemoteTerm/remoteterm' 100 || ln -sf '/opt/RemoteTerm/remoteterm' '/usr/bin/remoteterm'
else
    ln -sf '/opt/RemoteTerm/remoteterm' '/usr/bin/remoteterm'
fi

chmod 4755 '/opt/RemoteTerm/chrome-sandbox' || true

if hash update-mime-database 2>/dev/null; then
    update-mime-database /usr/share/mime || true
fi

if hash update-desktop-database 2>/dev/null; then
    update-desktop-database /usr/share/applications || true
fi
