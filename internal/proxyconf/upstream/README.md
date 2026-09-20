Vendored from official [`DNSCrypt/dnscrypt-proxy`](https://github.com/DNSCrypt/dnscrypt-proxy) tag in `VERSION`.

These files are **not** from the `78tacos/dnscrypt-proxy` fork. `config.go.txt` is the upstream `dnscrypt-proxy/config.go` renamed so `go test ./...` does not try to compile it.

Refresh:

```bash
./scripts/vendor-dnscrypt-schema.sh <tag>
go generate ./internal/proxyconf
```

Upstream license: ISC (see `LICENSE`).
