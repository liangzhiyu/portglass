//go:build darwin

package capture

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"github.com/ebitengine/purego"
)

// Check the actual system library and C struct layout without administrator
// privileges. Live capture is covered separately by --self-test.
func TestSystemLibpcapPacketABI(t *testing.T) {
	api, err := loadPCAP()
	if err != nil {
		t.Fatal(err)
	}
	lib, err := purego.Dlopen("/usr/lib/libpcap.A.dylib", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		t.Fatal(err)
	}
	var openOffline func(string, *byte) uintptr
	purego.RegisterLibFunc(&openOffline, lib, "pcap_open_offline")
	var file bytes.Buffer
	for _, v := range []any{uint32(0xa1b2c3d4), uint16(2), uint16(4), int32(0), uint32(0), uint32(65535), uint32(0), uint32(1800000000), uint32(123456), uint32(4), uint32(4), uint32(2)} {
		if err := binary.Write(&file, binary.LittleEndian, v); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(t.TempDir(), "fixture.pcap")
	if err := os.WriteFile(path, file.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	var errorBuffer [256]byte
	h := openOffline(path, &errorBuffer[0])
	if h == 0 {
		t.Fatalf("open: %s", errorBuffer)
	}
	defer api.close(h)
	var headerPtr, dataPtr unsafe.Pointer
	if rc := api.next(h, &headerPtr, &dataPtr); rc != 1 {
		t.Fatal("pcap_next_ex", rc)
	}
	header := *(*packetHeader)(headerPtr)
	if header.Seconds != 1800000000 || header.Micros != 123456 || header.Captured != 4 || header.Length != 4 {
		t.Fatalf("wrong native struct layout: %#v", header)
	}
	if !bytes.Equal(unsafe.Slice((*byte)(dataPtr), 4), []byte{2, 0, 0, 0}) {
		t.Fatal("wrong native payload")
	}
	if api.link(h) != 0 {
		t.Fatal("wrong link type")
	}
	if rc := api.next(h, &headerPtr, &dataPtr); rc != -2 {
		t.Fatal("expected end of PCAP", rc)
	}
}

func TestHelperRejectsUntrustedArguments(t *testing.T) {
	if RunHelper(nil) == nil {
		t.Fatal("accepted missing arguments")
	}
	if RunHelper([]string{"/tmp/untrusted.sock", "short", "lo0", "7010", "501", "20"}) == nil {
		t.Fatal("accepted invalid nonce")
	}
	if RunHelper([]string{"/tmp/untrusted.sock", string(bytes.Repeat([]byte("a"), 64)), "lo0", "7010", "-1", "20"}) == nil {
		t.Fatal("accepted invalid uid")
	}
}
