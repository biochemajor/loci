// Package sfx implements the loci self-extracting archive (SFX) format and the
// pack/extract logic shared by the builder (CLI and GUI) and the extractor stub.
//
// Output file layout produced by the builder:
//
//	[ extractor stub binary ]         native executable for the target OS
//	[ container ]                     magic + header(JSON) + blob
//	[ trailer ]                       fixed 24 bytes locating the container
//
// The stub, at run time, opens its own executable, reads the trailer at the end
// of the file, seeks to the container, parses the header, optionally decrypts,
// verifies checksums, unzips, and renames each file with the configured
// prefix/suffix.
package sfx

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// Magic markers. The trailing byte is a format version so future changes are
// detectable rather than silently misread.
var (
	containerMagic = []byte("LOCISFX\x01")
	trailerMagic   = []byte("LOCITRL\x01")
)

const (
	// FormatVersion is the on-disk format version recorded in the header.
	FormatVersion = 1

	magicLen   = 8
	trailerLen = 8 + 8 + magicLen // containerOffset(8) + containerLen(8) + magic(8)

	// KDFIterations is the PBKDF2-HMAC-SHA256 iteration count used when a pack
	// is password protected.
	KDFIterations = 200000
)

// FileEntry records one source file's identity and its checksum taken BEFORE
// compression, so the extractor can prove each extracted file matches the
// original bit-for-bit.
type FileEntry struct {
	// Name is the path stored inside the zip (forward-slash separated,
	// relative). The base element receives the prefix/suffix on extraction.
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Rename holds the configurable, independent prefix and suffix applied to each
// extracted file's base name. Suffix is inserted before the file extension.
type Rename struct {
	Prefix string `json:"prefix"`
	Suffix string `json:"suffix"`
}

// Header is the JSON metadata stored at the front of the container.
type Header struct {
	Version   int    `json:"version"`
	Encrypted bool   `json:"encrypted"`
	Cipher    string `json:"cipher,omitempty"` // e.g. "AES-256-GCM"
	KDF       string `json:"kdf,omitempty"`    // e.g. "PBKDF2-HMAC-SHA256"
	Iters     int    `json:"iters,omitempty"`
	Salt      string `json:"salt,omitempty"`  // base64, if encrypted
	Nonce     string `json:"nonce,omitempty"` // base64, if encrypted
	Rename    Rename `json:"rename"`
	// ArchiveSHA256 is the checksum of the zip AFTER compression (and before
	// any encryption). Verifies the integrity of the compressed archive.
	ArchiveSHA256 string      `json:"archiveSha256"`
	Files         []FileEntry `json:"files"`
}

// buildContainer assembles magic + headerJSON + blob into a single byte slice.
func buildContainer(headerJSON, blob []byte) []byte {
	out := make([]byte, 0, magicLen+4+len(headerJSON)+len(blob))
	out = append(out, containerMagic...)
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(headerJSON)))
	out = append(out, n[:]...)
	out = append(out, headerJSON...)
	out = append(out, blob...)
	return out
}

// parseContainer splits a container into its header JSON bytes and blob bytes.
func parseContainer(container []byte) (headerJSON, blob []byte, err error) {
	if len(container) < magicLen+4 {
		return nil, nil, errors.New("sfx: container too small")
	}
	if string(container[:magicLen]) != string(containerMagic) {
		return nil, nil, errors.New("sfx: bad container magic")
	}
	hlen := binary.BigEndian.Uint32(container[magicLen : magicLen+4])
	start := magicLen + 4
	end := start + int(hlen)
	if end > len(container) {
		return nil, nil, fmt.Errorf("sfx: header length %d exceeds container", hlen)
	}
	return container[start:end], container[end:], nil
}

// buildTrailer returns the fixed-size trailer that locates the container within
// the final output file.
func buildTrailer(containerOffset, containerLen uint64) []byte {
	t := make([]byte, trailerLen)
	binary.BigEndian.PutUint64(t[0:8], containerOffset)
	binary.BigEndian.PutUint64(t[8:16], containerLen)
	copy(t[16:], trailerMagic)
	return t
}

// parseTrailer reads a trailer's offset and length fields, validating the magic.
func parseTrailer(trailer []byte) (containerOffset, containerLen uint64, err error) {
	if len(trailer) != trailerLen {
		return 0, 0, fmt.Errorf("sfx: trailer must be %d bytes", trailerLen)
	}
	if string(trailer[16:]) != string(trailerMagic) {
		return 0, 0, errors.New("sfx: no loci archive found in this file")
	}
	return binary.BigEndian.Uint64(trailer[0:8]), binary.BigEndian.Uint64(trailer[8:16]), nil
}
