package sfx

import (
	"archive/zip"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/crypto/pbkdf2"
)

// Options configures a build.
type Options struct {
	// Sources is the list of files and/or directories to archive. Directories
	// are added recursively, preserving their internal structure under the
	// directory's own name.
	Sources []string
	// Rename is applied to each extracted file's base name.
	Rename Rename
	// Password, when non-empty, encrypts the compressed archive with
	// AES-256-GCM. When empty the archive is stored unencrypted.
	Password string
}

// Result summarizes what a build produced, for display to the user.
type Result struct {
	Files         []FileEntry
	ArchiveSHA256 string
	Encrypted     bool
	ContainerSize int
}

// BuildContainer zips the sources, computes before/after checksums, optionally
// encrypts, and returns the assembled container plus a summary. The container
// is stub-independent, so it is built once and reused for every target OS.
func BuildContainer(opts Options) ([]byte, Result, error) {
	if len(opts.Sources) == 0 {
		return nil, Result{}, fmt.Errorf("no source files selected")
	}

	entries, zipBytes, err := zipSources(opts.Sources)
	if err != nil {
		return nil, Result{}, err
	}

	// Checksum AFTER compression (of the plaintext zip).
	archiveSum := sha256.Sum256(zipBytes)
	archiveHex := hex.EncodeToString(archiveSum[:])

	h := Header{
		Version:       FormatVersion,
		Rename:        opts.Rename,
		ArchiveSHA256: archiveHex,
		Files:         entries,
	}

	blob := zipBytes
	if opts.Password != "" {
		salt := make([]byte, 16)
		if _, err := rand.Read(salt); err != nil {
			return nil, Result{}, fmt.Errorf("generating salt: %w", err)
		}
		key := pbkdf2.Key([]byte(opts.Password), salt, KDFIterations, 32, sha256.New)
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, Result{}, err
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			return nil, Result{}, err
		}
		nonce := make([]byte, gcm.NonceSize())
		if _, err := rand.Read(nonce); err != nil {
			return nil, Result{}, fmt.Errorf("generating nonce: %w", err)
		}
		blob = gcm.Seal(nil, nonce, zipBytes, nil)

		h.Encrypted = true
		h.Cipher = "AES-256-GCM"
		h.KDF = "PBKDF2-HMAC-SHA256"
		h.Iters = KDFIterations
		h.Salt = base64.StdEncoding.EncodeToString(salt)
		h.Nonce = base64.StdEncoding.EncodeToString(nonce)
	}

	headerJSON, err := json.Marshal(h)
	if err != nil {
		return nil, Result{}, err
	}
	container := buildContainer(headerJSON, blob)

	return container, Result{
		Files:         entries,
		ArchiveSHA256: archiveHex,
		Encrypted:     h.Encrypted,
		ContainerSize: len(container),
	}, nil
}

// zipSources builds the zip archive in memory and returns the manifest of
// per-file (pre-compression) checksums alongside the zip bytes.
func zipSources(sources []string) ([]FileEntry, []byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	var entries []FileEntry
	seen := map[string]bool{}

	add := func(zipName string, info os.FileInfo, path string) error {
		zipName = filepath.ToSlash(zipName)
		if seen[zipName] {
			return fmt.Errorf("duplicate entry %q (two sources map to the same name)", zipName)
		}
		seen[zipName] = true

		hdr, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		hdr.Name = zipName
		hdr.Method = zip.Deflate
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		hasher := sha256.New()
		if _, err := io.Copy(w, io.TeeReader(f, hasher)); err != nil {
			return err
		}
		entries = append(entries, FileEntry{
			Name:   zipName,
			Size:   info.Size(),
			SHA256: hex.EncodeToString(hasher.Sum(nil)),
		})
		return nil
	}

	for _, src := range sources {
		src = filepath.Clean(src)
		info, err := os.Stat(src)
		if err != nil {
			return nil, nil, fmt.Errorf("reading %q: %w", src, err)
		}
		if info.IsDir() {
			parent := filepath.Dir(src)
			err := filepath.Walk(src, func(path string, fi os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if fi.IsDir() {
					return nil // zip stores files; directories are implicit
				}
				if !fi.Mode().IsRegular() {
					return nil // skip symlinks, devices, etc.
				}
				rel, err := filepath.Rel(parent, path)
				if err != nil {
					return err
				}
				return add(rel, fi, path)
			})
			if err != nil {
				return nil, nil, err
			}
		} else {
			if !info.Mode().IsRegular() {
				return nil, nil, fmt.Errorf("%q is not a regular file", src)
			}
			if err := add(filepath.Base(src), info, src); err != nil {
				return nil, nil, err
			}
		}
	}

	if err := zw.Close(); err != nil {
		return nil, nil, err
	}
	if len(entries) == 0 {
		return nil, nil, fmt.Errorf("no regular files found in the selected sources")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, buf.Bytes(), nil
}

// WriteSFX writes a self-extracting output: the stub binary, then the shared
// container, then a trailer locating it. unixExec marks the file executable
// (for macOS/Linux stubs); Windows .exe files do not need the bit.
func WriteSFX(stub, container []byte, outPath string, unixExec bool) error {
	mode := os.FileMode(0o644)
	if unixExec {
		mode = 0o755
	}
	f, err := os.OpenFile(outPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Write(stub); err != nil {
		return err
	}
	if _, err := f.Write(container); err != nil {
		return err
	}
	trailer := buildTrailer(uint64(len(stub)), uint64(len(container)))
	if _, err := f.Write(trailer); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	// Ensure the executable bit survives any umask on unix targets.
	if unixExec {
		if err := os.Chmod(outPath, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// SafeRename applies the configured prefix and suffix to a single path,
// operating only on the final (base) element and inserting the suffix before
// the file extension. It rejects results that would escape via path separators.
func SafeRename(name string, r Rename) (string, error) {
	slash := filepath.ToSlash(name)
	dir := ""
	base := slash
	if i := strings.LastIndex(slash, "/"); i >= 0 {
		dir = slash[:i+1]
		base = slash[i+1:]
	}
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	renamed := r.Prefix + stem + r.Suffix + ext
	if strings.ContainsAny(renamed, `/\`) || renamed == "" {
		return "", fmt.Errorf("invalid rename result %q", renamed)
	}
	return dir + renamed, nil
}
