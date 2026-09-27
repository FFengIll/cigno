package pkg

import (
	"bufio"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/ulikunitz/xz"
)

// Compression detection mirrors moby/go-archive DetectCompression:
// identification is by content magic bytes, never by file extension.

func decompressReader(r io.Reader, magic []byte) (io.Reader, string, error) {
	switch {
	case len(magic) >= 2 && magic[0] == 0x1f && magic[1] == 0x8b:
		gr, err := gzip.NewReader(r)
		if err != nil {
			return nil, "", fmt.Errorf("opening gzip stream: %w", err)
		}
		return gr, "gzip", nil
	case len(magic) >= 3 && string(magic[:3]) == "BZh":
		return bzip2.NewReader(r), "bzip2", nil
	case len(magic) >= 6 && string(magic[:6]) == "\xfd7zXZ\x00":
		xr, err := xz.NewReader(r)
		if err != nil {
			return nil, "", fmt.Errorf("opening xz stream: %w", err)
		}
		return xr, "xz", nil
	default:
		return r, "none", nil
	}
}

// decompressFile opens path and returns a reader over the decompressed
// content plus the detected compression name.
func decompressFile(path string) (io.ReadCloser, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	br := bufio.NewReader(f)
	magic, err := br.Peek(6)
	if err != nil && len(magic) == 0 {
		f.Close()
		return nil, "", fmt.Errorf("reading %s: %w", path, err)
	}
	inner, name, err := decompressReader(br, magic)
	if err != nil {
		f.Close()
		return nil, "", err
	}
	return struct {
		io.Reader
		io.Closer
	}{Reader: inner, Closer: f}, name, nil
}

// probeTar reads the first 512-byte block of r, reports whether the
// (already decompressed) stream is a tar archive, and returns a reader
// replaying everything it consumed.
func probeTar(r io.Reader) (io.Reader, bool, error) {
	block := make([]byte, 512)
	n, err := io.ReadFull(r, block)
	if err == io.EOF || (err == io.ErrUnexpectedEOF && n == 0) {
		return bytes.NewReader(block[:n]), false, nil // empty file
	}
	if err != nil && err != io.ErrUnexpectedEOF {
		return nil, false, err
	}

	// checksum verification covers ustar and pre-POSIX archives alike
	var isTar bool
	if n == 512 {
		// the stored checksum is 6 octal digits NUL-padded then space-terminated
		field := strings.Trim(string(block[148:156]), "\x00 ")
		if stored, err := strconv.ParseInt(field, 8, 64); err == nil {
			var sum int64
			for i := 0; i < 512; i++ {
				b := block[i]
				if i >= 148 && i < 156 {
					b = ' '
				}
				sum += int64(b)
			}
			isTar = stored == sum
		}
	}
	if isTar {
		return io.MultiReader(bytes.NewReader(block), r), true, nil
	}
	return bytes.NewReader(block), false, nil
}

// openArchiveTar opens path, decompresses it (by magic bytes) and probes
// for a tar stream. Returns (replaying reader, true, nil) for tar archives,
// or (nil, false, nil) when the file is not a tar archive.
func openArchiveTar(path string) (io.ReadCloser, bool, error) {
	dec, _, err := decompressFile(path)
	if err != nil {
		return nil, false, err
	}
	probed, isTar, err := probeTar(dec)
	if err != nil {
		dec.Close()
		return nil, false, err
	}
	if !isTar {
		dec.Close()
		return nil, false, nil
	}
	return struct {
		io.Reader
		io.Closer
	}{Reader: probed, Closer: dec}, true, nil
}
