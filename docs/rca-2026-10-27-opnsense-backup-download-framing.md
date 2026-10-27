# Root Cause Analysis — OPNsense 26.7.7 backup download sends more body bytes than the declared `Content-Length`

**Status:** narrowed down. The HTTPS frontend on the affected appliance is **lighttpd** (not nginx),
and disabling FreeBSD **kTLS** fixes the bug. **Caddy** is the proposed permanent replacement.
Hostnames are sanitized (`fw03.example.net`); the rest is verbatim from the incident.

| | |
|---|---|
| Date of incident | 2026-10-27 10:51:40 (client clock) |
| Reporter | `opnborg` v0.1.233, Go 1.27.x, stdlib `net/http`, HTTP/1.1 over TLS 1.3 |
| Fleet | 8 OPNsense 26.7.7 appliances, identical client + TLS parameters, polled in parallel |
| Affected member | `fw03.example.net:8443` only — the other 7 succeed every pass |
| Endpoint | `GET /api/core/backup/download/this` (API key auth, HTTPS `:8443`) |
| Server | lighttpd + PHP/Phalcon, FreeBSD kTLS enabled |

## 1. Symptom (verbatim client log, trimmed)

```log
[OPNBORG-API][BACKUP][START][SERVER] fw03.example.net:8443      <-- affected
2026/10/27 10:51:40 Unsolicited response received on idle HTTP channel starting with "</server_address>
        <agent_name/> ... <syscheck><enabled>1</enabled></syscheck> ... <act"; err=<nil>
[OPNBORG-API][FETCH][ERROR][FAIL:XML-VALIDATION] https://fw03.example.net:8443/api/core/backup/download/this
[OPNBORG-API][BACKUP][ERROR][FAIL:UNABLE-TO-FETCH-XML] fw03.example.net:8443[INVALID-XML-FILE]
... (the 7 unaffected members all returned a valid, unchanged document in the same second)
```

The unsolicited tail is the `<wazuh>` section of the same `config.xml` backup — a byte-for-byte
continuation of the failed response, not a second HTTP response and not garbage. The client had
already read exactly the declared `Content-Length` and completed cleanly with a 2xx status, so the
document was cut mid-XML at precisely the split point. That is an RFC 9112 §6.3 message-framing
violation: the server declared `Content-Length: N` but wrote `M > N` body octets. `err=<nil>` on the
unsolicited read shows the TLS layer delivered the bytes intact — the framing is wrong, not the
encryption.

## 2. Root cause

**lighttpd on FreeBSD with kernel TLS (kTLS) enabled emits a `Content-Length` smaller than the number
of body octets it then writes.** The declared prefix is delivered as a truncated, therefore invalid,
XML document; the surplus tail of the same document is written to the same TLS record stream and
surfaces on the pooled keep-alive connection. The split falls wherever the declared length ends —
arbitrarily inside `<wazuh>`.

A/B evidence: disabling kTLS on the appliance (`kern.ipc.tls.enable=0` in `/boot/loader.conf`, then
reboot) fixes the bug. With kTLS enabled the failure reproduces **on and off** (intermittently)
against fw03 only; the other seven identical members never trip it. That empirical result promotes
the kTLS send-path length/short-write anomaly from a low-ranked candidate to the leading cause — a
clean byte-continuation is exactly what a length desync in the offload path looks like when the
surplus lands on the next read of the pooled connection.

**Not nginx:** the affected appliance's HTTPS frontend is lighttpd. Earlier nginx-directed checks
(`nginx -V`, `ssl_ktls` directives, nginx access-log fields) do not apply to this case.

## 3. Impact

- The affected appliance's backup pass fails; retries usually recover it on a later attempt.
- A partial document must never be treated as a valid backup: the client rejects it as
  `[FAIL:XML-VALIDATION]` and records no archive/commit for that pass.
- Silent corruption risk for any client that trusts the declared `Content-Length` instead of
  validating the XML: it would consume a truncated config and miss the surplus tail.

## 4. Mitigation

Appliance-side (fixes it): disable kTLS in `/boot/loader.conf`:

```
kern.ipc.tls.enable=0
```

reboot, then confirm `sysctl net.inet.tcp.tls` reports `0`. As a client-side fallback the daemon
already accepts `OPN_TLS_MAX_VERSION=1.2`.

Client-side (already shipped): backup XML downloads send `Connection: close` and retry on a fresh
TCP+TLS connection (3 attempts), so a surplus-framed stream cannot poison a pooled connection and a
transient pass recovers while staying on TLS 1.3. Each attempt logs its own diagnosis
(`[STATUS] … [DECLARED] N [RECEIVED] M`); attach that line to a future occurrence before filing
upstream.

## 5. Permanent fix proposal

**Replace lighttpd with Caddy** as the HTTPS frontend on affected appliances. Caddy's HTTP/1.1
framing and Go-based TLS stack avoid the FreeBSD kTLS send path that lighttpd trips here, and it
brings automatic certificate handling. Alternatively, keep lighttpd and fix or disable its kTLS path
upstream. Until either lands, `kern.ipc.tls.enable=0` is the supported workaround.

## 6. Server-side verification checklist

```sh
opnsense-version -v; uname -a; freebsd-version -k; freebsd-version -r
sysctl net.inet.tcp.tls
lighttpd -v

# single-request framing probe (run repeatedly, also while changing the config):
curl -skv --http1.1 --tlsv1.3 -u '<apikey>:<apisecret>' \
     https://fw03.example.net:8443/api/core/backup/download/this -o /tmp/body.xml
grep -i 'content-length' /tmp/curl_headers   # compare against:
stat -c%s /tmp/body.xml
```

## 7. References

- RFC 9112, *HTTP/1.1*, §6.3 Message Length — the framing rule violated here
- Go standard library `net/http`, `transport.go`, `persistConn.readLoop` — the unsolicited-response
  log source and its keep-alive idle-pool semantics
- OPNsense core, `BackupController` download action — `GET /api/core/backup/download/this`
- FreeBSD `tcp(4)` kernel TLS offload (`net.inet.tcp.tls`) — leading cause; disable via
  `kern.ipc.tls.enable=0`
- lighttpd — affected HTTPS frontend; Caddy — proposed replacement
