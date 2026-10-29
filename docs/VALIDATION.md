# PortGlass 0.1.0 validation

Validation host: Apple Silicon, macOS 27.0, Go 1.27.3, MyGo 0.7.2.

Completed locally:

- `CGO_ENABLED=0 go test ./...`: passed.
- `CGO_ENABLED=0 go vet ./...`: passed.
- Real system libpcap offline-file read: native function bindings, packet-header layout and packet bytes passed.
- HTTP tests: TCP segmentation, reordered and repeated segments, pipelined pairing, chunked gzip, informational responses, HEAD, close-delimited bodies, live response previews, raw TCP and TLS detection.
- UI tests: search, detail formats, clear, invalid port validation, history eviction, wide/compact light/dark layouts.
- Native window launch and content capture: passed.
- ARM64 app bundle build, ad-hoc signature verification and DMG checksum verification: passed.

Pending:

- `PortGlass --self-test`: real IPv4 / IPv6 loopback capture across two temporary HTTP ports. The system administrator-authorization dialog timed out without authorization. No live capture success is claimed.
- Public GitHub repository and Release publication: deferred until the live capture verification is resolved.

The screenshots use explicitly labeled demonstration data. No real user traffic is included in this repository.
