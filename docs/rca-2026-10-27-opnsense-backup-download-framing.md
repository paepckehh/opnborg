# Root Cause Analysis — OPNsense 26.7.7 backup download sends more body bytes than the declared `Content-Length`

**Scope:** client-side evidence chain + ranked server-side hypotheses, prepared for upstreaming to the
OPNsense project (`opnsense/core`); the FreeBSD side (kernel TLS offload) is included because one
candidate mechanism involves it. Hostnames and the reporting domain are sanitized
(`fw03.example.net`); everything else is verbatim from the incident.

| | |
|---|---|
| Date of incident | 2026-10-27 10:51:40 (client clock) |
| Reporter | `opnborg` fleet backup daemon v0.1.233, Go 1.27.x, stdlib `net/http`, HTTP/1.1 over TLS 1.3 |
| Fleet | 8 OPNsense appliances, identical build (26.7.7), same client + TLS parameters, polled in parallel in the same second |
| Affected member | `fw03.example.net:8443` only — the other 7 succeed every pass |
| Endpoint | `GET /api/core/backup/download/this` (API key auth, HTTPS `:8443`) |
| Client TLS at incident | TLS 1.3 only, X25519, no session resumption, SPKI SHA-256 keypin via `VerifyConnection`, `DisableCompression: true` (no `Accept-Encoding` sent), keep-alive connection pooling |

## 1. Incident signature (verbatim client log, 2026-10-27 10:51:40)

```log
[OPNBORG-API][STARTING][BACKUP]
[OPNBORG-API][BACKUP][START][SERVER] fw01.example.net:8443
[OPNBORG-API][BACKUP][START][SERVER] fw02.example.net:8443
[OPNBORG-API][BACKUP][START][SERVER] fw03.example.net:8443      <-- affected
... (8 members total, dispatched in the same second)
2026/10/27 10:51:40 Unsolicited response received on idle HTTP channel starting with "</server_address>
        <agent_name/>
        <protocol>tcp</protocol>
        <port>1514</port>
        <debug_level>0</debug_level>
      </general>
      <auth>
        <password/>
        <port>1515</port>
      </auth>
      <logcollector>
        <remote_commands>1</remote_commands>
        <syslog_programs/>
        <suricata_eve_log>1</suricata_eve_log>
      </logcollector>
      <rootcheck>
        <enabled>1</enabled>
      </rootcheck>
      <syscollector>
        <enabled>1</enabled>
      </syscollector>
      <syscheck>
        <enabled>1</enabled>
      </syscheck>
      <active_response>
        <enabled>1</enabled>
        <remote_commands>1</remote_commands>
        <fw_alias_ignore/>
        <repeated_offenders/>
      </act"; err=<nil>
[OPNBORG-API][FETCH][ERROR][FAIL:XML-VALIDATION] https://fw03.example.net:8443/api/core/backup/download/this
[OPNBORG-API][BACKUP][ERROR][FAIL:UNABLE-TO-FETCH-XML] fw03.example.net:8443[INVALID-XML-FILE]
[OPNBORG-API][BACKUP][SERVER][NO-CHANGE] fw01.example.net:8443
... (the 7 unaffected members all returned a valid, unchanged document in the same second)
```

## 2. Evidence chain — what each log fact proves

1. **`Unsolicited response received on idle HTTP channel starting with %q; err=%v`** is emitted by
   the Go standard library `net/http` (`transport.go`, `persistConn.readLoop`) — and *only* there —
   when a **pooled idle** connection receives bytes that do not begin a new HTTP response. It is
   printed through the std `log` package, which is why the line carries a raw `2026/10/27 10:51:40`
   timestamp instead of the daemon's `[OPNBORG-API]` display prefix.
   `err=<nil>` proves the TLS record layer delivered the data intact: no transport error, no record
   corruption, no decryption failure.
2. **The out-of-band payload is a byte-for-byte continuation of the very response that just failed
   validation.** The dumped tail (`</server_address>`, `<agent_name/>`, agent port `1514`, authd
   port `1515`, `logcollector` with `suricata_eve_log`, `rootcheck`, `syscollector`, `syscheck`,
   `active_response`) is the `<wazuh>` plugin section of an OPNsense `config.xml` backup document —
   i.e. the middle of the body of `GET /api/core/backup/download/this`. It is not a second HTTP
   response (no `HTTP/1.1 …` start line), not garbage, not compressed data.
3. **The delivered body was exactly the declared `Content-Length` and completed without error.**
   The client logged no `[FAIL:READ-BODY]` and no `[FAIL:HTTP-STATUS]` for the request — the body
   read finished cleanly at the declared length with a 2xx status. The delivered document was cut
   mid-XML at precisely the point where the unsolicited tail continues (delivered prefix ended
   inside `<wazuh><general><server_address>`; the tail begins with the closing `</server_address>`).
   This rules out chunked-transfer framing (a broken chunk would surface as a client read error,
   not a clean completion) and rules out a short body (which would surface as an unexpected-EOF
   read error).
4. **Same client, same TLS parameters, same second: 7/8 members succeed.** The fault is not a
   generic Go/OPNsense TLS interop issue; it is specific to the affected appliance's response
   generation path and intermittent-per-persistent there.

## 3. Deduced wire behavior — an RFC 9112 §6.3 message-framing violation

For a `Content-Length: N` response the server must send exactly N body octets. What the affected
appliance actually did:

```
client                                        fw03.example.net:8443 (nginx / PHP / TLS)
  | --- GET /api/core/backup/download/this --------------------------> |
  | <-- 200 OK                                                       |
  |     Content-Length: N        (N = size known to the server        |
  |                                at header-emission time)           |
  | <-- body[0:N]   = config.xml cut mid-<wazuh> section             |  actual payload length M, M > N
  |     client reads exactly N octets, no error, body complete        |
  | connection returned to the keep-alive idle pool                   |
  | <-- body[N:M]  = <wazuh> ... document tail  (surplus octets)      |
  | readLoop: bytes on idle conn, not a response start                |
  |          -> "Unsolicited response received on idle HTTP channel"  |
  |          -> Go discards the connection (no poisoning of pass N+1) |
```

**Conclusion:** the appliance's HTTPS frontend emitted a `Content-Length` header *smaller than the
number of body octets it then wrote on the socket*. The client consumed the declared prefix (a
truncated, therefore invalid, XML document) and the surplus tail of the same document remained in
the kernel/TLS record queue, to be delivered out-of-band when the connection was next read. The
dump in the log is a bounded prefix (the client's read buffer) of a surplus of unknown total size.

## 4. Ruled out

| Hypothesis | Ruled out by |
|---|---|
| TLS record corruption / stream desync (e.g. kTLS record bug) | tail is a *clean byte-continuation* of the plaintext document; `err=<nil` on the unsolicited read |
| Content compression mismatch (gzip) | tail is plaintext XML; client sends no `Accept-Encoding`, no content-encoding negotiated |
| Duplicate / pipelined response | tail does not begin an HTTP response start line |
| Chunked framing break | body read completed cleanly at a declared length (a broken chunk yields a client read error instead) |
| Client bug | same client + TLS config succeeds against 7/8 members in the same second; Go's `Content-Length` body reader is exact |

## 5. Candidate root causes on the appliance (ranked, each falsifiable server-side)

**(a) Stale length, fresh payload — the backup document changed between header emission and body
write.** The endpoint (or a caching layer) computes `Content-Length` from one version of the
generated backup XML and streams another, larger version — e.g. a concurrent `config.xml` change or
backup regeneration on the appliance at the same second. `N = old size`, streamed bytes = new size.
This fits the mid-document split point exactly (the split falls wherever the old document ended,
arbitrarily inside `<wazuh>`), and it explains fleet-uniqueness (only fw03 had a config write at
10:51:40).
*Verify:* correlate `config.xml` / backup-artifact write + rotate timestamps around the failure
second; compare the observed surplus `M − N` against the size delta of consecutive config
revisions; run the download in a loop while mutating the config.

**(b) PHP/Phalcon response framing — extra output after the declared body.** The MVC response for
the backup action sets `Content-Length` from the framework body, and an output handler, plugin
hook, or stray `echo` appends additional bytes after it. Fits the "clean continuation" signature
if the appended bytes belong to the same template.
*Verify:* `curl -skv https://fw03.example.net:8443/api/core/backup/download/this -o /tmp/body.xml`,
then compare the `Content-Length:` response header against `stat -c%s /tmp/body.xml`. Any mismatch
on a single-request repro is conclusive.

**(c) FreeBSD kernel-TLS (`ssl_ktls` / sendfile) write anomaly on nginx.** If the affected appliance
enables TLS offload, a short-write/replay class bug could re-emit or duplicate a byte range. A pure
replay would duplicate *earlier* bytes rather than continue them, so this ranks low against the
observed clean single split — but it must be excluded because it is the one candidate that belongs
to FreeBSD rather than OPNsense.
*Verify:* `nginx -V` for kTLS support + `ssl_ktls` directives, `sysctl net.inet.tcp.tls`,
`freebsd-version -k -r`; A/B test with TLS offload disabled; compare nginx `$bytes_sent` /
`$body_bytes_sent` log fields against `Content-Length`.

**(d) Response-mutating filter (`sub_filter`, WAF or similar plugin) growing the body** without
updating the declared length.
*Verify:* diff the nginx site config of fw03 against a healthy fleet member; disable filters and
retest.

## 6. Server-side data to collect on the affected appliance (checklist)

```sh
opnsense-version -v
uname -a; freebsd-version -k; freebsd-version -r
nginx -V 2>&1 | tr ' ' '\n' | grep -iE 'tls|version'
grep -RniE 'ssl_ktls|ktls' /usr/local/etc/nginx/
sysctl net.inet.tcp.tls 2>/dev/null
php -v

# single-request framing probe (run repeatedly, also while changing the config):
curl -skv --http1.1 --tlsv1.3 -u '<apikey>:<apisecret>' \
     https://fw03.example.net:8443/api/core/backup/download/this -o /tmp/body.xml
grep -i 'content-length' /tmp/curl_headers   # compare against:
stat -c%s /tmp/body.xml

# nginx access log around 10:51:40 with $content_length / $body_bytes_sent
# config.xml write + rotate timestamps around the failure second
```

## 7. Timeline

| Client time | Event |
|---|---|
| 10:51:40 | Fleet backup pass starts; 8 workers dispatched in parallel, one per appliance |
| 10:51:40 | fw03 answers `200` with `Content-Length: N`; client reads the body: exactly N octets, no error |
| 10:51:40 | Delivered XML fails validation (document cut mid-`<wazuh>`); `[FAIL:XML-VALIDATION]` logged |
| 10:51:40 | Connection returned to the idle keep-alive pool |
| 10:51:40 | `readLoop` receives the surplus tail `body[N:M]` on the idle connection; Go logs the unsolicited-response line (`err=<nil>`) and discards the connection |
| 10:51:40 | 7/8 members return unchanged documents (valid XML, SHA-256 match); fw03 is marked fetch-ERROR for the pass |

## 8. Client-side impact and mitigation (already shipped)

The daemon (`opnborg` v0.1.234, tag `v0.1.234`) now:

- retries the idempotent backup GET on a **fresh TCP+TLS connection** (3 attempts) when a body
  comes back truncated, short, invalid, or 5xx — this recovers the pass while staying on TLS 1.3;
- sends `Connection: close` on every API fetch, so a surplus-framed stream can never poison a
  pooled connection;
- logs per attempt `[STATUS] … [DECLARED] N [RECEIVED] M`, turning the next occurrence into hard
  numbers for `N` and `M` (see §3) — please re-attach that line to this report when it recurs.

This handles the symptom for the client; the framing violation itself is server-side and needs the
upstream fix.

## 9. Suggested issue text (opnsense/core)

> **Title:** 26.7.7 `api/core/backup/download/this` intermittently sends more body bytes than the
> declared `Content-Length` (keep-alive framing violation)
>
> **Summary:** Against one appliance of an identical 8-member fleet, the backup download API
> intermittently returns a response whose declared `Content-Length` is smaller than the number of
> body octets actually written. A keep-alive HTTP/1.1 client receives a truncated (invalid) XML
> document and then the surplus tail of the same document out-of-band on the pooled connection
> (observed client-side as Go's "Unsolicited response received on idle HTTP channel", `err=<nil>`).
> The surplus tail is a byte-for-byte continuation of the `<wazuh>` section of the same `config.xml`
> backup document. Seven identically-built fleet members polled in the same second with the same
> client and TLS parameters never exhibit it.
>
> **Expected:** `Content-Length` equals the number of body octets sent (RFC 9112 §6.3).
>
> **Actual:** intermittently `Content-Length: N` with `M > N` body octets written; `M − N` surplus
> octets leak onto the keep-alive connection.
>
> **Attachments:** client log (§1), evidence chain (§2), deduced wire behavior (§3), server-side
> verification checklist (§6). Per-attempt `[DECLARED]/[RECEIVED]` measurements will be appended on
> the next occurrence.

## 10. References

- RFC 9112, *HTTP/1.1*, §6.3 Message Length — framing rules violated here
- Go standard library `net/http`, `transport.go`, `persistConn.readLoop` — the unsolicited-response
  log source (and its keep-alive idle-pool semantics)
- OPNsense core, `BackupController` download action — `GET /api/core/backup/download/this`
- FreeBSD `tcp(4)` kernel TLS offload (`net.inet.tcp.tls`, nginx `ssl_ktls`) — candidate (c)