#!/usr/bin/env bash
mininet="$(dirname ${BASH_SOURCE[0]})"
set -e

SERVER_GW="$1"
SERVERS="$@"
KEYS=/tmp/server_keys
SSH_TYPE="-t ssh-ed25519"
SSH_ID=~/.ssh/id_rsa
if [ -f /etc/issue ]; then
	echo Issue exists
	if grep -q "Debian.*7" /etc/issue; then
		SSH_TYPE=""
	fi
fi

if [ ! -f $SSH_ID ]; then
	echo "Creating global key"
	echo -e '\n\n\n\n' | ssh-keygen
fi

rm -f $KEYS
for s in $SERVERS; do
	port=22
	if `echo $s| grep -q : `; then
	  port=`echo $s| awk -F: '{ print $2 }' `
	  s=`echo $s| awk -F: '{ print $1 }' `
	fi
	echo Starting to install on $s, port $port
	login=root@$s
	ip=$( host $s | sed -e "s/.* //" )
	ping -c 2 -t 1 -i 0.3 $ip || ( echo "Server $s is not pingable. Stopping."; exit 1 )
	ssh-keygen -R $s > /dev/null 2>&1 || true
	ssh-keygen -R $ip  > /dev/null 2>&1 || true
	ssh-keyscan $SSH_TYPE -p $port $s >> ~/.ssh/known_hosts 2> /dev/null || (echo "Server $s is not running sshd yet. Stopping."; exit 1)
	ssh-copy-id -f -i $SSH_ID -p $port $login &> /dev/null || (echo "Server $s is not running sshd yet. Stopping."; exit 1)
	ssh -p $port $login "test ! -f .ssh/id_rsa && echo -e '\n\n\n\n' | ssh-keygen > /dev/null 2>&1" || true
	ssh -p $port $login cat .ssh/id_rsa.pub >> $KEYS
	if ! ssh -p $port $login "egrep -q '(14.04|16.04|18.04|20.04|Debian GNU/Linux 8)' /etc/issue"; then
		clear
		echo "$s does not have Ubuntu 14.04, 16.04, 18.04, 20.04 or Debian 8 installed - aborting"
		exit 1
	fi

	echo "Running mininet install in the background."
	scp -P $port $mininet/install_mininet.sh $login: > /dev/null
	ssh -p $port -f $login "./install_mininet.sh > install.log 2>&1"

done

DONE=false
while [ "$DONE" != "true" ]; do
    if [ -n "`pgrep -f install_mininet.sh`" ]; then
	  echo
	  echo "$( date ) - Waiting for background installs to finish:"
	  ps -ef | grep install_mininet.sh
	else
	  DONE=true
    fi
	sleep 2
done

echo -e "\nAll servers are done installing - copying ssh-keys"

rm -f server_list
port=22
if `echo $SERVER_GW | grep -q : `; then
  port=`echo $SERVER_GW | awk -F: '{ print $2 }' `
  SERVER_GW=`echo $SERVER_GW | awk -F: '{ print $1 }' `
fi
for s in $SERVERS; do
    port2=22
	if `echo $s| grep -q : `; then
	  port2=`echo $s| awk -F: '{ print $2 }' `
	  s=`echo $s| awk -F: '{ print $1 }' `
	fi
	login=root@$s
	cat $KEYS | ssh -p $port $login "cat - >> .ssh/authorized_keys"
	ip=$( host $s | sed -e "s/.* //" )
	ssh -p $port root@$SERVER_GW "ssh-keyscan -p $port2 $SSH_TYPE $s >> .ssh/known_hosts 2> /dev/null"
	ssh -p $port root@$SERVER_GW "ssh-keyscan -p $port2 $SSH_TYPE $ip >> .ssh/known_hosts 2> /dev/null"
	echo $s >> server_list
done

echo "Done installing to:"
cat server_list
rm $KEYS
