package sfx

import (
	"archive/zip"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/pbkdf2"
)

// ErrWrongPassword is returned when decryption authentication fails, which for
// AES-GCM means either a wrong password or a tampered archive.
var ErrWrongPassword = errors.New("wrong password or corrupted archive")

// Archive is a loaded SFX payload ready to extract.
type Archive struct {
	Header Header
	blob   []byte
}

// Self loads the archive embedded in the currently running executable.
func Self() (*Archive, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	// Resolve symlinks so os.Args[0]-style launches still find the real file.
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return Load(exe)
}

// Load reads and parses the SFX container from the given file.
func Load(path string) (*Archive, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	size, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, err
	}
	if size < int64(trailerLen) {
		return nil, errors.New("sfx: no loci archive found in this file")
	}

	trailer := make([]byte, trailerLen)
	if _, err := f.ReadAt(trailer, size-int64(trailerLen)); err != nil {
		return nil, err
	}
	off, clen, err := parseTrailer(trailer)
	if err != nil {
		return nil, err
	}
	if int64(off)+int64(clen) > size-int64(trailerLen) {
		return nil, errors.New("sfx: archive trailer points outside the file")
	}

	container := make([]byte, clen)
	if _, err := f.ReadAt(container, int64(off)); err != nil {
		return nil, err
	}
	headerJSON, blob, err := parseContainer(container)
	if err != nil {
		return nil, err
	}
	var h Header
	if err := json.Unmarshal(headerJSON, &h); err != nil {
		return nil, fmt.Errorf("sfx: parsing header: %w", err)
	}
	if h.Version != FormatVersion {
		return nil, fmt.Errorf("sfx: unsupported archive version %d", h.Version)
	}
	return &Archive{Header: h, blob: blob}, nil
}

// ExtractedFile records the outcome for one extracted file.
type ExtractedFile struct {
	Original string // name as stored in the archive
	Written  string // path written to disk (after prefix/suffix)
	Verified bool   // checksum matched the pre-compression manifest
	SizeOK   bool
}

// ExtractResult summarizes an extraction.
type ExtractResult struct {
	DestDir string
	Files   []ExtractedFile
}

// Decrypt returns the plaintext zip bytes, applying the password only when the
// archive is encrypted. It also verifies the post-compression archive checksum.
func (a *Archive) Decrypt(password string) ([]byte, error) {
	zipBytes := a.blob
	if a.Header.Encrypted {
		salt, err := base64.StdEncoding.DecodeString(a.Header.Salt)
		if err != nil {
			return nil, fmt.Errorf("sfx: bad salt: %w", err)
		}
		nonce, err := base64.StdEncoding.DecodeString(a.Header.Nonce)
		if err != nil {
			return nil, fmt.Errorf("sfx: bad nonce: %w", err)
		}
		iters := a.Header.Iters
		if iters <= 0 {
			iters = KDFIterations
		}
		key := pbkdf2.Key([]byte(password), salt, iters, 32, sha256.New)
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, err
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			return nil, err
		}
		pt, err := gcm.Open(nil, nonce, a.blob, nil)
		if err != nil {
			return nil, ErrWrongPassword
		}
		zipBytes = pt
	}

	sum := sha256.Sum256(zipBytes)
	if hex.EncodeToString(sum[:]) != a.Header.ArchiveSHA256 {
		return nil, errors.New("sfx: archive checksum mismatch (corrupted download)")
	}
	return zipBytes, nil
}

// Extract decrypts (if needed), unzips into destDir, applies the configured
// prefix/suffix rename, and verifies each file against the pre-compression
// checksum. Existing files in destDir are overwritten.
func (a *Archive) Extract(destDir, password string) (*ExtractResult, error) {
	zipBytes, err := a.Decrypt(password)
	if err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, fmt.Errorf("reading archive: %w", err)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, err
	}
	absDest, err := filepath.Abs(destDir)
	if err != nil {
		return nil, err
	}

	manifest := make(map[string]FileEntry, len(a.Header.Files))
	for _, e := range a.Header.Files {
		manifest[e.Name] = e
	}

	res := &ExtractResult{DestDir: absDest}
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		orig := filepath.ToSlash(zf.Name)
		if strings.Contains(orig, "..") || strings.HasPrefix(orig, "/") {
			return nil, fmt.Errorf("unsafe path in archive: %q", orig)
		}
		renamed, err := SafeRename(orig, a.Header.Rename)
		if err != nil {
			return nil, err
		}
		target := filepath.Join(absDest, filepath.FromSlash(renamed))
		// Defense in depth against path traversal after joining.
		if target != absDest && !strings.HasPrefix(target, absDest+string(os.PathSeparator)) {
			return nil, fmt.Errorf("unsafe extraction path for %q", orig)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil, err
		}

		hasher := sha256.New()
		rc, err := zf.Open()
		if err != nil {
			return nil, err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			rc.Close()
			return nil, err
		}
		if _, err := io.Copy(out, io.TeeReader(rc, hasher)); err != nil {
			rc.Close()
			out.Close()
			return nil, err
		}
		rc.Close()
		if err := out.Close(); err != nil {
			return nil, err
		}

		ef := ExtractedFile{Original: orig, Written: target}
		if e, ok := manifest[orig]; ok {
			ef.Verified = hex.EncodeToString(hasher.Sum(nil)) == e.SHA256
			ef.SizeOK = zf.FileInfo().Size() == e.Size
		}
		res.Files = append(res.Files, ef)
	}
	return res, nil
}
