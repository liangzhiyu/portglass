# PortGlass 0.1.0 validation

Validation host: Apple Silicon, macOS 27.0, Go 1.27.3, MyGo 0.7.2. Verified on 2026-10-29 / 2026-10-30 (Asia/Shanghai).

Completed locally:

- `CGO_ENABLED=0 go test ./...`: passed.
- `CGO_ENABLED=0 go vet ./...`: passed.
- Real system libpcap offline-file read: native function bindings, packet-header layout and packet bytes passed.
- HTTP tests: TCP segmentation, reordered and repeated segments, pipelined pairing, chunked gzip, informational responses, HEAD, close-delimited bodies, live response previews, raw TCP and TLS detection.
- UI tests: search, detail formats, copy, protocol navigation, keyboard request navigation, appearance switching, clear, invalid port validation, history eviction, compact control bounds, high-contrast fallback, wide/compact light/dark layouts.
- Native window launch and content capture: passed.
- Real IPv4 and IPv6 loopback capture on two temporary HTTP ports: passed through the packaged app's system authorization dialog, BPF helper, TCP assembly and request/response pairing. Both request and response payloads were checked.
- ARM64 app bundle build, ad-hoc signature verification and DMG checksum verification: passed.

- Redesigned native window: light/dark glass, empty state and Headers inspected through macOS screenshots and accessibility. Light glass was corrected to sample an opaque in-window backdrop; native output was rechecked.
- `MYGO_FRAME_STATS=all`: the packaged app reported GPU rendering. In the final launch, the first frame took 30.6 ms, and the second static frame 1.3 ms. This is not a sustained load benchmark.

The capture implementation was unchanged by the visual redesign. Automatic tests do not claim HTTPS decryption, HTTP/2 decoding, IP-fragment reassembly or a full VoiceOver audit.

The screenshots use explicitly labeled demonstration data. No real user traffic is included in this repository.
