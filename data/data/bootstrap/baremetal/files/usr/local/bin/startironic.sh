#!/bin/bash

set -ex

IRONIC_IMAGE=${IRONIC_IMAGE:-"quay.io/metal3-io/ironic:master"}
IRONIC_INSPECTOR_IMAGE=${IRONIC_INSPECTOR_IMAGE:-"quay.io/metal3-io/ironic-inspector:master"}
IPA_DOWNLOADER_IMAGE=${IPA_DOWNLOADER_IMAGE:-"quay.io/metal3-io/ironic-ipa-downloader:master"}
COREOS_DOWNLOADER_IMAGE=${COREOS_DOWNLOADER_IMAGE:-"quay.io/openshift-metal3/rhcos-downloader:master"}

# FIXME this should be provided by the installer
RHCOS_IMAGE_URL="https://releases-art-rhcos.svc.ci.openshift.org/art/storage/releases/ootpa/410.8.20190520.0"

# First we stop any previously started containers, because ExecStop only runs when the ExecStart process
# e.g this script is still running, but we exit if *any* of the containers exits unexpectedly
for name in ironic-api ironic-conductor ironic-exporter ironic-inspector dnsmasq httpd mariadb ipa-downloader coreos-downloader; do
    podman ps | grep -w "$name$" && podman kill $name
    podman ps --all | grep -w "$name$" && podman rm $name -f
done

# Start the provisioning nic if not already started
# FIXME we should detect the nic in the ironic container (currently relies on the ifname, and won't pick 
# up the description added here)
# FIXME The IP/subnet and gateway should be provided via survey input?
# FIXME: the ironic containers currently bind to all interfaces, which is not secure in the
# case of the cluster, but we'll need the installer to access the public nic on the boostrap node
# so probably we'll need a forwarding rule after https://github.com/metal3-io/ironic-image/pull/56
PROVISIONING_NIC=ens4
if ! nmcli -t device | grep "$PROVISIONING_NIC:ethernet:connected:provisioning"; then
    nmcli c add type ethernet ifname $PROVISIONING_NIC con-name provisioning ip4 172.22.0.2/24 gw4 172.22.0.1
    nmcli c up provisioning
fi

# Wait for the interface to come up
# This is how the ironic container currently detects IRONIC_IP, this could probably be improved by using
# nmcli show provisioning there instead, but we need to confirm that works with the static-ip-manager
while [ -z "$(ip -4 address show dev "$PROVISIONING_NIC" | grep -oP '(?<=inet\s)\d+(\.\d+){3}' | head -n 1)" ]; do
    sleep 1
done

# set password for mariadb
mariadb_password=$(echo $(date;hostname)|sha256sum |cut -c-20)

IRONIC_SHARED_VOLUME="ironic"
podman volume create $IRONIC_SHARED_VOLUME || true

# Apparently network-online doesn't necessarily mean iptables is ready, so wait until it is..
while ! iptables -L; do
  sleep 1
done

# Add firewall rules to ensure the IPA ramdisk can reach httpd, Ironic and the Inspector API on the host
for port in 80 5050 6385 ; do
    if ! sudo iptables -C INPUT -i $PROVISIONING_NIC -p tcp -m tcp --dport $port -j ACCEPT > /dev/null 2>&1; then
        sudo iptables -I INPUT -i $PROVISIONING_NIC -p tcp -m tcp --dport $port -j ACCEPT
    fi
done

# Start dnsmasq, http, mariadb, and ironic containers using same image
# Currently we do this outside of a pod because we need to ensure the images
# are downloaded before starting the API pods
podman run -d --net host --privileged --name mariadb \
     -v $IRONIC_SHARED_VOLUME:/shared:z --entrypoint /bin/runmariadb \
     --env MARIADB_PASSWORD=$mariadb_password ${IRONIC_IMAGE}

podman run -d --net host --privileged --name dnsmasq \
     --env PROVISIONING_INTERFACE=$PROVISIONING_NIC \
     -v $IRONIC_SHARED_VOLUME:/shared:z --entrypoint /bin/rundnsmasq ${IRONIC_IMAGE}

podman run -d --net host --privileged --name httpd \
     --env PROVISIONING_INTERFACE=$PROVISIONING_NIC \
     -v $IRONIC_SHARED_VOLUME:/shared:z --entrypoint /bin/runhttpd ${IRONIC_IMAGE}

podman run -d --net host --name ipa-downloader \
     --env CACHEURL="http://172.22.0.1/images" \
     -v $IRONIC_SHARED_VOLUME:/shared:z ${IPA_DOWNLOADER_IMAGE} /usr/local/bin/get-resource.sh

podman run -d --net host --name coreos-downloader \
     --env CACHEURL="http://172.22.0.1/images" \
     -v $IRONIC_SHARED_VOLUME:/shared:z ${COREOS_DOWNLOADER_IMAGE} /usr/local/bin/get-resource.sh $RHCOS_IMAGE_URL

# Start the conductor so db sync can happen while images download
sudo podman run -d --net host --privileged --name ironic-conductor \
     --env MARIADB_PASSWORD=$mariadb_password \
     --env PROVISIONING_INTERFACE=$PROVISIONING_NIC \
     --env OS_CONDUCTOR__HEARTBEAT_TIMEOUT=120 \
     --entrypoint /bin/runironic-conductor \
     -v $IRONIC_SHARED_VOLUME:/shared:z ${IRONIC_IMAGE}

# Wait for images to be downloaded/ready
while ! curl --fail http://localhost:80/images/rhcos-ootpa-latest.qcow2.md5sum ; do sleep 1 ; done

podman run -d --net host --privileged --name ironic-inspector \
     --env PROVISIONING_INTERFACE=$PROVISIONING_NIC \
     -v $IRONIC_SHARED_VOLUME:/shared:z "${IRONIC_INSPECTOR_IMAGE}"

sudo podman run -d --net host --privileged --name ironic-api \
     --env MARIADB_PASSWORD=$mariadb_password \
     --env PROVISIONING_INTERFACE=$PROVISIONING_NIC \
     --entrypoint /bin/runironic-api \
     -v $IRONIC_SHARED_VOLUME:/shared:z ${IRONIC_IMAGE}

sudo podman run -d --net host --privileged --name ironic-exporter \
     --entrypoint /bin/runironic-exporter \
     -v $IRONIC_SHARED_VOLUME:/shared:z ${IRONIC_IMAGE}

while true; do
    for name in ironic-api ironic-conductor ironic-exporter ironic-inspector dnsmasq httpd mariadb; do
        podman ps | grep -w "$name$" || exit 1
    done
    sleep 10
done
