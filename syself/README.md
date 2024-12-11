# Keep our fork up-to-date with upstream

In most cases it is best to use the upstream source code.

If you took files from upstrea, you need to update the import statements in Go:

```sh
fd  '.*.go' --exec sd '"github.com/hetznercloud/hcloud-cloud-controller-manager/' '"github.com/syself/hetzner-cloud-controller-manager/'
```

Then:

```sh
go mod tidy
```

Check for hetznercloud in go.mod:

```
grep hetznercloud go.mod
```

Only this line is ok:

> github.com/hetznercloud/hcloud-go/v2 ...
