# Kubernetes Cloud Controller Manager for Hetzner Cloud & Hetzner Dedicated

[![GitHub Actions status](https://github.com/syself/hetzner-cloud-controller-manager/workflows/Run%20tests/badge.svg)](https://github.com/syself/hetzner-cloud-controller-manager/actions)

The Syself Hetzner Cloud [cloud-controller-manager](https://kubernetes.io/docs/concepts/architecture/cloud-controller/) integrates your Kubernetes cluster with the Hetzner Cloud & Robot APIs.

This project is a fork of the [HCloud CCM](https://github.com/hetznercloud/hcloud-cloud-controller-manager) maintained by [Syself](https://syself.com)

## Features

- **Node**:
  - Updates your `Node` objects with information about the server from the Cloud & Robot API.
  - Instance Type, Location, Datacenter, Server ID, IPs.
- **Node Lifecycle**:
  - Cleans up stale `Node` objects when the server is deleted in the API.
- **Routes** (if enabled):
  - Routes traffic to the pods through Hetzner Cloud Networks. Removes one layer of indirection in CNIs that support this.
- **Load Balancer**:
  - Watches Services with `type: LoadBalancer` and creates Hetzner Cloud Load Balancers for them, adds Kubernetes Nodes as targets for the Load Balancer.

Read more about cloud controllers in the [Kubernetes documentation](https://kubernetes.io/docs/tasks/administer-cluster/running-cloud-controller/).

### Node Metadata Example

```yaml
apiVersion: v1
kind: Node
metadata:
  labels:
    node.kubernetes.io/instance-type: cx22
    topology.kubernetes.io/region: fsn1
    topology.kubernetes.io/zone: fsn1-dc8
    instance.hetzner.cloud/provided-by: cloud
  name: node
spec:
  podCIDR: 10.244.0.0/24
  providerID: hcloud://123456 # <-- Hetzner Cloud Server ID
status:
  addresses:
    - address: node
      type: Hostname
    - address: 1.2.3.4 # <-- Hetzner Cloud Server public ipv4
      type: ExternalIP
```

## Deployment

This deployment example uses `kubeadm` to bootstrap an Kubernetes
cluster, with [flannel](https://github.com/coreos/flannel) as overlay
network agent. Feel free to adapt the steps to your preferred method of
installing Kubernetes.

These deployment instructions are designed to guide with the
installation of the `hetzner-cloud-controller-manager` and are by no
means an in depth tutorial of setting up Kubernetes clusters.
**Previous knowledge about the involved components is required.**

Please refer to the [kubeadm cluster creation
guide](https://kubernetes.io/docs/setup/independent/create-cluster-kubeadm/),
which these instructions are meant to augment and the [kubeadm
documentation](https://kubernetes.io/docs/reference/setup-tools/kubeadm/kubeadm/).

1. The cloud controller manager adds the labels when a node is added to
   the cluster. For current Kubernetes versions, this means we
   have to add the `--cloud-provider=external` flag to the `kubelet`. How you
   do this depends on your Kubernetes distribution. With `kubeadm` you can
   either set it in the kubeadm config
   ([`nodeRegistration.kubeletExtraArgs`][kubeadm-config]) or through a systemd
   drop-in unit `/etc/systemd/system/kubelet.service.d/20-hcloud.conf`:

   ```ini
   [Service]
   Environment="KUBELET_EXTRA_ARGS=--cloud-provider=external"
   ```

   Note: the `--cloud-provider` flag is deprecated since K8S 1.19. You
   will see a log message regarding this. For now (v1.31) it is still required.

2. Now the control plane can be initialized:

   ```sh
   sudo kubeadm init --pod-network-cidr=10.244.0.0/16
   ```

3. Configure kubectl to connect to the kube-apiserver:

   ```sh
   mkdir -p $HOME/.kube
   sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
   sudo chown $(id -u):$(id -g) $HOME/.kube/config
   ```

4. Deploy the flannel CNI plugin:

TODO: Update docs to use Cilium.

   ```sh
   kubectl apply -f https://github.com/flannel-io/flannel/releases/latest/download/kube-flannel.yml
   ```

5. Patch the flannel deployment to tolerate the `uninitialized` taint:

   ```sh
   kubectl -n kube-system patch ds kube-flannel-ds --type json -p '[{"op":"add","path":"/spec/template/spec/tolerations/-","value":{"key":"node.cloudprovider.kubernetes.io/uninitialized","value":"true","effect":"NoSchedule"}}]'
   ```

6. Create a secret containing your Hetzner Cloud API token.

   ```sh
   kubectl -n kube-system create secret generic hcloud --from-literal=token=<hcloud API token>
   ```

7. Deploy the `hetzner-cloud-controller-manager`:

   **Using Helm (recommended):**

   ```
   helm repo add hcloud https://charts.hetzner.cloud
   helm repo update hcloud
   helm install hccm hcloud/hcloud-cloud-controller-manager -n kube-system
   ```

   See the [Helm chart README](./chart/README.md) for more info.

   **Legacy installation method**:

    ```sh
    kubectl apply -f https://github.com/syself/hetzner-cloud-controller-manager/releases/latest/download/ccm.yaml
    ```

[kubeadm-config]: https://kubernetes.io/docs/reference/config-api/kubeadm-config.v1beta4/#kubeadm-k8s-io-v1beta4-NodeRegistrationOptions

## Networks support

When you use the Cloud Controller Manager with networks support, the CCM is in favor of allocating the IPs (& setup the
routing) (Docs: <https://kubernetes.io/docs/concepts/architecture/cloud-controller/#route-controller>). The CNI plugin you
use needs to support this k8s native functionality (Cilium does it, I don't know about Calico & WeaveNet), so basically
you use the Hetzner Cloud Networks as the underlying networking stack.

When you use the CCM without Networks support it just disables the RouteController part, all other parts work completely
the same. Then just the CNI is in charge of making all the networking stack things. Using the CCM with Networks support
has the benefit that your node is connected to a private network so the node doesn't need to encrypt the connections and
you have a bit less operational overhead as you don't need to manage the Network.

If you want to use the Hetzner Cloud `Networks` Feature, head over to
the [Deployment with Networks support
documentation](./docs/deploy_with_networks.md).

If you manage the network yourself it might still be required to let the CCM know about private networks. You can do
this by adding the environment variable
with the network name/ID in the CCM deployment.

```
          env:
            - name: HCLOUD_NETWORK
              valueFrom:
                secretKeyRef:
                  name: hcloud
                  key: network
```

You also need to add the network name/ID to the
secret: `kubectl -n kube-system create secret generic hcloud --from-literal=token=<hcloud API token> --from-literal=network=<hcloud Network_ID_or_Name>`
.

## Kube-proxy mode IPVS and HCloud LoadBalancer

If `kube-proxy` is run in IPVS mode, the `Service` manifest needs to have the
annotation `load-balancer.hetzner.cloud/hostname` where the FQDN resolves to the HCloud LoadBalancer IP.

See <https://github.com/syself/hetzner-cloud-controller-manager/issues/212>

## Versioning policy

We aim to support the latest three versions of Kubernetes. When a Kubernetes
version is marked as _End Of Life_, we will stop support for it and remove the
version from our CI tests. This does not necessarily mean that the
Cloud Controller Manager does not still work with this version. We will
not fix bugs related only to an unsupported version.

Current Kubernetes Releases: <https://kubernetes.io/releases/>


## Development

### Setup a development environment

To set up a development environment, make sure you installed the following tools:

- [tofu](https://opentofu.org/)
- [k3sup](https://github.com/alexellis/k3sup)
- [docker](https://www.docker.com/)
- [skaffold](https://skaffold.dev/)

1. Configure a `HCLOUD_TOKEN` in your shell session.

> [!WARNING]
> The development environment runs on Hetzner Cloud servers which will induce costs.

2. Deploy the development cluster:

```sh
make -C dev up
```

3. Load the generated configuration to access the development cluster:

```sh
source dev/files/env.sh
```

4. Check that the development cluster is healthy:

```sh
kubectl get nodes -o wide
```

5. Start developing hcloud-cloud-controller-manager in the development cluster:

```sh
skaffold dev
```

On code change, skaffold will rebuild the image, redeploy it and print all logs.

⚠️ Do not forget to clean up the development cluster once are finished:

```sh
make -C dev down
```

### Run the unit tests

To run the unit tests, make sure you installed the following tools:

- [Go](https://go.dev/)

1. Run the following command to run the unit tests:

```sh
go test ./...
```

### Run the kubernetes e2e tests

Before running the e2e tests, make sure you followed the [Setup a development environment](#setup-a-development-environment) steps.

1. Run the kubernetes e2e tests using the following command:

```bash
alias k="kubectl"
alias ksy="kubectl -n kube-system"
alias kgp="kubectl get pods"
alias kgs="kubectl get services"
```

## Local test setup

This repository provides [skaffold](https://skaffold.dev/) to easily deploy / debug this controller on demand

### Requirements

1. Install [hcloud-cli](https://github.com/hetznercloud/cli)
2. Install [k3sup](https://github.com/alexellis/k3sup)
3. Install [cilium](https://github.com/cilium/cilium-cli)
4. Install [docker](https://www.docker.com/)

You will also need to set a `HCLOUD_TOKEN` in your shell session

### Manual Installation guide

1. Create an SSH key

Assuming you already have created an ssh key via `ssh-keygen`

```
hcloud ssh-key create --name ssh-key-ccm-test --public-key-from-file ~/.ssh/id_rsa.pub
```

2. Create a server

```
hcloud server create --name ccm-test-server --image ubuntu-20.04 --ssh-key ssh-key-ccm-test --type cx11
```

3. Setup k3s on this server

```
k3sup install --ip $(hcloud server ip ccm-test-server) --local-path=/tmp/kubeconfig --cluster --k3s-channel=v1.23 --k3s-extra-args='--no-flannel --no-deploy=servicelb --no-deploy=traefik --disable-cloud-controller --disable-network-policy --kubelet-arg=cloud-provider=external'
```

- The kubeconfig will be created under `/tmp/kubeconfig`

- Kubernetes version can be configured via `--k3s-channel`

4. Switch your kubeconfig to the test cluster. Very important: exporting this like

```
export KUBECONFIG=/tmp/kubeconfig
```

5. Install cilium + test your cluster

```
cilium install
```

6. Add your secret to the cluster

```
kubectl -n kube-system create secret generic hcloud --from-literal="token=$HCLOUD_TOKEN"
```

7. Deploy the hcloud-cloud-controller-manager

```
SKAFFOLD_DEFAULT_REPO=your_docker_hub_username skaffold dev
```

- `docker login` required
- Skaffold is using your own Docker Hub repo to push the HCCM image.
- After the first run, you might need to set the image to "public" on hub.docker.com

On code change, Skaffold will repack the image & deploy it to your test cluster again. It will also stream logs from the hccm Deployment.

_After setting this up, only the command from step 7 is required!_=

## License

Apache License, Version 2.0
