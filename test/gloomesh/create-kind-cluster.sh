#!/usr/bin/env bash

echo '>>> Create kind cluster'
number=$1
name=$2
region=$3
zone=$4
twodigits=$(printf "%02d\n" $number)

if [ -z "$3" ]; then
  region=us-east-1
fi

if [ -z "$4" ]; then
  zone=us-east-1a
fi

if hostname -I 2>/dev/null; then
  myip=$(hostname -I | awk '{ print $1 }')
else
  myip=$(ipconfig getifaddr en0)
fi

cat << EOF > /tmp/kind${number}.yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraPortMappings:
  - containerPort: 6443
    hostPort: 70${twodigits}
networking:
  serviceSubnet: "10.2${twodigits}.0.0/16"
  podSubnet: "10.1${twodigits}.0.0/16"
kubeadmConfigPatches:
- |
  apiVersion: kubeadm.k8s.io/v1beta2
  kind: ClusterConfiguration
  apiServer:
    extraArgs:
      service-account-signing-key-file: /etc/kubernetes/pki/sa.key
      service-account-key-file: /etc/kubernetes/pki/sa.pub
      service-account-issuer: api
      service-account-api-audiences: api,vault,factors
  metadata:
    name: config
- |
  kind: InitConfiguration
  nodeRegistration:
    kubeletExtraArgs:
      node-labels: "ingress-ready=true,topology.kubernetes.io/region=${region},topology.kubernetes.io/zone=${zone}"
EOF

kind create cluster --name kind${number} --config /tmp/kind${number}.yaml

ipkind=$(docker inspect kind${number}-control-plane | jq -r '.[0].NetworkSettings.Networks[].IPAddress')
networkkind=$(echo ${ipkind} | awk -F. '{ print $1"."$2 }')

kubectl --context=kind-kind${number} apply -f https://raw.githubusercontent.com/metallb/metallb/v0.9.3/manifests/namespace.yaml
kubectl --context=kind-kind${number} apply -f https://raw.githubusercontent.com/metallb/metallb/v0.9.3/manifests/metallb.yaml
kubectl --context=kind-kind${number} create secret generic -n metallb-system memberlist --from-literal=secretkey="$(openssl rand -base64 128)"

cat << EOF > /tmp/metallb${number}.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  namespace: metallb-system
  name: config
data:
  config: |
    address-pools:
    - name: default
      protocol: layer2
      addresses:
      - ${networkkind}.0${twodigits}.1-${networkkind}.0${twodigits}.254
EOF

kubectl --context=kind-kind${number} apply -f /tmp/metallb${number}.yaml

kubectl config delete-context ${name}
kubectl config rename-context kind-kind${number} ${name}

echo '>>> Load test flagger image into the kind cluster'
kind load docker-image test/flagger:latest --name kind1

rm /tmp/metallb1.yaml
rm /tmp/kind1.yaml