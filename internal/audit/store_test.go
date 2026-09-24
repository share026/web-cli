package audit

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

func TestDecompressAllBrowserEncodings(t *testing.T) {
	want := []byte(strings.Repeat("<html>web-cli source</html>\n", 50))
	var gz, fl, br bytes.Buffer
	gw := gzip.NewWriter(&gz)
	gw.Write(want)
	gw.Close()
	fw, _ := flate.NewWriter(&fl, flate.BestCompression)
	fw.Write(want)
	fw.Close()
	bw := brotli.NewWriter(&br)
	bw.Write(want)
	bw.Close()
	ze, _ := zstd.NewWriter(nil)
	zs := ze.EncodeAll(want, nil)
	for enc, body := range map[string][]byte{"gzip": gz.Bytes(), "deflate": fl.Bytes(), "br": br.Bytes(), "zstd": zs} {
		got, used, ok := Decompress(body, enc)
		if !ok || used != enc || !bytes.Equal(got, want) {
			t.Fatalf("%s: ok=%v used=%q equal=%v", enc, ok, used, bytes.Equal(got, want))
		}
	}
	if _, _, ok := Decompress(want, ""); ok {
		t.Fatal("identity must not be decoded")
	}
}
