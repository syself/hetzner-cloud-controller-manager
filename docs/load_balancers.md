# Load Balancers

Load Balancer support is implemented in the Cloud Controller as of
version v1.6.0. For using the Hetzner Cloud Load Balancers you need to
deploy a `Service` of type `LoadBalancer`.

## Sample Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: example-service
  annotations:
    load-balancer.hetzner.cloud/location: hel1
spec:
  selector:
    app: example
  ports:
    - port: 80
      targetPort: 8080
  type: LoadBalancer
```

The sample service will create a Load Balancer in the location `hel1`
with a service with `listen_port = 80` and `destination_port = 8080`. So
every traffic that arrives at the Load Balancer on Port 80 will be
routed to the public interface of the targets on port 8080.  You can
change the behavior of the Load Balancer by specifying more annotations.
A list of all available annotations can be found on
[pkg.go.dev](https://pkg.go.dev/github.com/syself/hetzner-cloud-controller-manager/internal/annotation#Name).
If you have the cloud controller deployed with Private Network Support,
we attach the Load Balancer to the specific network automatically. You
can specify with an annotation that the Load Balancer should use the
private network instead of the public network.

## Proxy Protocol

To enable proxy protocol on a Load Balancer service, set the
`load-balancer.hetzner.cloud/uses-proxyprotocol` annotation to `"true"`.
The service status then uses `ipMode: Proxy` for the Load Balancer
ingress entries.

```yaml
apiVersion: v1
kind: Service
metadata:
  name: example-service
  annotations:
    load-balancer.hetzner.cloud/uses-proxyprotocol: "true"
spec:
  type: LoadBalancer
```

Related docs:

* [Load balancer IP address
  mode](https://kubernetes.io/docs/concepts/services-networking/service/#load-balancer-ip-mode)
* [Kubernetes 1.29: New (alpha) Feature, Load Balancer IP Mode for
  Services](https://kubernetes.io/blog/2023/12/18/kubernetes-1-29-feature-loadbalancer-ip-mode-alpha/)

## Sample Service with Networks

```yaml
apiVersion: v1
kind: Service
metadata:
  name: example-service
  annotations:
    load-balancer.hetzner.cloud/location: hel1
    load-balancer.hetzner.cloud/use-private-ip: "true"
spec:
  selector:
    app: example
  ports:
    - port: 80
      targetPort: 8080
  type: LoadBalancer
```

For IPVS based plugins (kube-router, kube-proxy in ipvs mode, etc...) make sure you
supply '**load-balancer.hetzner.cloud/disable-private-ingress: "true"**' annotation
to your service or set "**HCLOUD_LOAD_BALANCERS_DISABLE_PRIVATE_INGRESS**" environment variable
to true on hcloud-cloud-controller-manager deployment as mentioned in a paragraph below. Otherwise, network
plugin installs load balancer's IP address on system's dummy interface effectively
looping IPVS system in a cycle. In such scenario cluster nodes won't ever pass load balancer's health probes

## Cluster-wide Defaults

For convenience, you can set the following environment variables as cluster-wide defaults, so you don't have to set them on each load balancer service. If a load balancer service has the corresponding annotation set, it overrides the default.

* `HCLOUD_LOAD_BALANCERS_LOCATION` (mutually exclusive with `HCLOUD_LOAD_BALANCERS_NETWORK_ZONE`)
* `HCLOUD_LOAD_BALANCERS_NETWORK_ZONE` (mutually exclusive with `HCLOUD_LOAD_BALANCERS_LOCATION`)
* `HCLOUD_LOAD_BALANCERS_DISABLE_PRIVATE_INGRESS`
* `HCLOUD_LOAD_BALANCERS_USE_PRIVATE_IP`
* `HCLOUD_LOAD_BALANCERS_ENABLED`
* `HCLOUD_LOAD_BALANCERS_ROBOT_TARGET_ADDRESS_FAMILY`
* `HCLOUD_LOAD_BALANCERS_RESTRICT_NODE_PORT_SOURCE`

## Targets for dedicated servers

A dedicated server has no server ID the Load Balancer can point at, so it is
added as an IP target.
`HCLOUD_LOAD_BALANCERS_ROBOT_TARGET_ADDRESS_FAMILY` picks which address is used,
and the `load-balancer.hetzner.cloud/robot-target-address-family` annotation
overrides it per service. One of `ipv4`, `ipv6` or `dualstack`, default `ipv4`.

Pick the family the cluster network carries. The Load Balancer reaches a node
over IPv6 only if the pod network and the node ports are up on IPv6, so on an
IPv4 cluster an IPv6 target never passes its health check.

`dualstack` adds both addresses of the same server. That registers the server
twice, so it counts twice against the target limit of the Load Balancer type and
takes a double share of the traffic compared to a cloud server. Use it only if
you really want both.

This setting is separate from `HCLOUD_LOAD_BALANCERS_DISABLE_IPV6`, which
controls the public IPv6 address of the Load Balancer itself.

## Limit the NodePort to the Load Balancer

A Service of type `LoadBalancer` also gets a NodePort, and the Load Balancer
sends its traffic to that NodePort on every node. The NodePort is open on every
node address, not only to the Load Balancer. If the nodes have public IPs and no
private network or firewall in front of them, anyone on the internet can reach
the Service directly on `<node-ip>:<nodePort>`, bypassing the Load Balancer.

With `HCLOUD_LOAD_BALANCERS_RESTRICT_NODE_PORT_SOURCE=true`, or the
`load-balancer.hetzner.cloud/restrict-node-port-source: "true"` annotation on a
Service, the cloud controller manager adds the Load Balancer's own IPs to the
Service's `spec.loadBalancerSourceRanges`, as `/32` entries:

* the public IPv4, if the public network is enabled;
* the private IPs, if the Load Balancer is attached to a network.

The Load Balancer's IPv6 address is not added. The Load Balancer connects to the
nodes over IPv4 even when a client uses IPv6.

Entries already on the Service are kept. The entries the cloud controller
manager added are recorded in the read-only
`load-balancer.hetzner.cloud/node-port-source-ranges` annotation. With that, it
replaces them when the Load Balancer's IP changes, and removes them when the
annotation is set to `"false"` or the Service stops being of type `LoadBalancer`.
The annotation overrides the environment variable in both directions.

The cloud controller manager only writes the field. Your Service proxy has to
apply it to the NodePort:

* **Cilium** with kube-proxy replacement applies it to the NodePort when
  `bpf.lbSourceRangeAllTypes=true` is set. In-cluster clients are not affected,
  because Cilium only checks traffic that arrives on the node's network
  interface.
* **kube-proxy** does not apply it to the NodePort. It also applies it to
  in-cluster traffic to the Load Balancer IP, so pods calling the Service
  through that IP are dropped. Do not enable this feature with kube-proxy.

The Load Balancer opens its own connection to the node, so the node only ever
sees the Load Balancer's IP as the source. Client CIDRs in
`loadBalancerSourceRanges` therefore never match. To filter by client IP, enable
the [Proxy Protocol](#proxy-protocol) and filter in the ingress controller.

The NodePort is open from the moment the Service is created until the Load
Balancer exists and its IP has been written, usually seconds.

## Reference existing Load Balancers

If you already have a Load Balancer that you want to use in Kubernetes, for
example if you provisioned it in Terraform, you can specify these two
annotations:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: example-service
  annotations:
    load-balancer.hetzner.cloud/location: hel1
    load-balancer.hetzner.cloud/name: your-existing-lb-name
```

The Load Balancer will then be adopted by the hcloud-cloud-controller-manager,
and the services and targets are set up for your cluster.

If you delete this `Service` in Kubernetes, the hcloud-cloud-controller-manager
will delete the associated Load Balancer. If the Load Balancer is managed
through Terraform, this causes problems. To disable this, you can enable
deletion protection on the Load Balancer, this way hcloud-cloud-controller-manager
will just skip deleting it when the associated `Service` is deleted.
