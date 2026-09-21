// Command stub is the extractor embedded at the front of every loci
// self-extracting archive. When run, it reads the archive appended to its own
// executable, optionally asks for a password, extracts the files (renamed with
// the configured prefix/suffix), and verifies every file against the checksum
// taken before compression.
//
// It is built once per target OS/arch by build/build-stubs.sh and embedded into
// the builder, which appends the payload at pack time.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/biochemajor/loci/internal/sfx"
	"golang.org/x/term"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "\nError:", err)
		pause()
		os.Exit(1)
	}
	pause()
}

func run() error {
	arc, err := sfx.Self()
	if err != nil {
		return err
	}

	exe, _ := os.Executable()
	base := strings.TrimSuffix(filepath.Base(exe), filepath.Ext(exe))
	destDir := filepath.Join(filepath.Dir(exe), base+"_extracted")

	fmt.Printf("loci self-extracting archive\n")
	fmt.Printf("  files:      %d\n", len(arc.Header.Files))
	fmt.Printf("  encrypted:  %v\n", arc.Header.Encrypted)
	if arc.Header.Rename.Prefix != "" || arc.Header.Rename.Suffix != "" {
		fmt.Printf("  rename:     prefix=%q suffix=%q\n", arc.Header.Rename.Prefix, arc.Header.Rename.Suffix)
	}
	fmt.Printf("  extract to: %s\n\n", destDir)

	password := ""
	if arc.Header.Encrypted {
		var res *sfx.ExtractResult
		for attempt := 1; attempt <= 3; attempt++ {
			password, err = readPassword()
			if err != nil {
				return err
			}
			res, err = arc.Extract(destDir, password)
			if errors.Is(err, sfx.ErrWrongPassword) {
				fmt.Fprintf(os.Stderr, "  %v (%d attempt(s) left)\n", err, 3-attempt)
				continue
			}
			if err != nil {
				return err
			}
			return report(res)
		}
		return errors.New("too many incorrect password attempts")
	}

	res, err := arc.Extract(destDir, password)
	if err != nil {
		return err
	}
	return report(res)
}

func report(res *sfx.ExtractResult) error {
	verified, failed := 0, 0
	for _, f := range res.Files {
		if f.Verified {
			verified++
		} else {
			failed++
			fmt.Fprintf(os.Stderr, "  ! checksum mismatch: %s\n", f.Original)
		}
	}
	fmt.Printf("\nExtracted %d file(s) to:\n  %s\n", len(res.Files), res.DestDir)
	fmt.Printf("Checksum verified: %d/%d\n", verified, len(res.Files))
	if failed > 0 {
		return fmt.Errorf("%d file(s) failed checksum verification", failed)
	}
	return nil
}

func readPassword() (string, error) {
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		fmt.Print("Password: ")
		b, err := term.ReadPassword(fd)
		fmt.Println()
		return string(b), err
	}
	fmt.Print("Password: ")
	s, err := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimRight(s, "\r\n"), err
}

// pause keeps a double-clicked console window open long enough to read output.
func pause() {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return
	}
	fmt.Print("\nPress Enter to close...")
	bufio.NewReader(os.Stdin).ReadString('\n')
}
