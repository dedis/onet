#!/usr/bin/env bash

export DEBIAN_FRONTEND="noninteractive"

# Package installation
echo 'export LC_ALL="en_US.UTF-8"' >> /etc/environment
. /etc/environment
apt-get update
apt-get install -y screen rsync software-properties-common git vim cifs-utils pv htop mtr \
    golang aufs-tools ca-certificates xz-utils \
    debootstrap lxc rinse psmisc

# Configure ssh-port-forwarding
echo GatewayPorts yes >> /etc/ssh/sshd_config
service ssh restart

case "$( cat /etc/issue )" in
*14.04*)
    echo "Installing for Ubuntu 14.04"
    apt-get install -y btrfs-tools
    # Mininet installation
    git clone git://github.com/mininet/mininet
    cd mininet || exit 1
    git checkout 2.2.1
    ./util/install.sh -a
    ;;
*16.04*)
    echo "Installing for Ubuntu 16.04"
    apt-get install -y btrfs-tools mininet openvswitch-testcontroller
    cp /usr/bin/ovs-testcontroller /usr/bin/ovs-controller
    ;;
*18.04*)
    echo "Installing for Ubuntu 18.04"
    apt-get install -y btrfs-tools mininet openvswitch-testcontroller
    for a in stop disable; do
      systemctl $a openvswitch-testcontroller
    done
    ;;
*20.04*)
    echo "Installing for Ubuntu 20.04"
    # Mininet installation
    git clone git://github.com/mininet/mininet
    cd mininet || exit 1
    ./util/install.sh -a
    apt install -y openvswitch-testcontroller python-is-python3
    for a in stop disable; do
      systemctl $a openvswitch-testcontroller
    done
    service openvswitch-switch start
    ;;
*)
    echo "Unknown system - only know Ubuntu 1404, 1604, 1804 and 2004!"
    ;;
esac
