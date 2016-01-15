package compress

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"

	"github.com/disorganizer/brig/util/testutil"
)

func testDecAndCompress(t *testing.T, size int64) {
	path := testutil.CreateFile(size)

	compressedPath := path + ".pack"
	decompressedPath := path + ".unpack"

	defer os.Remove(path)
	defer os.Remove(compressedPath)
	defer os.Remove(decompressedPath)

	if _, err := CompressFile(path, compressedPath); err != nil {
		t.Errorf("File compression failed: %v", err)
		return
	}

	if _, err := DecompressFile(compressedPath, decompressedPath); err != nil {
		t.Errorf("File decompression failed: %v", err)
		return
	}

	a, _ := ioutil.ReadFile(path)
	b, _ := ioutil.ReadFile(decompressedPath)
	c, _ := ioutil.ReadFile(compressedPath)

	if !bytes.Equal(a, b) {
		t.Errorf("Source and decompressed not equal")
	}

	if bytes.Equal(a, c) && size != 0 {
		t.Errorf("Source was not compressed (same as source)")
	}
}

func TestDecAndCompress(t *testing.T) {
	sizes := []int64{0, 1, 1024, 1024 * 1024}
	for _, size := range sizes {
		testDecAndCompress(t, size)
	}
}

func BenchmarkCompress(b *testing.B) {
	for n := 0; n < b.N; n++ {
		testDecAndCompress(nil, 1024*1024*10)
	}
}