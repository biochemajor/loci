// Command loci-cli builds self-extracting archives from the command line. It is
// the fully scriptable twin of the loci GUI and shares the same core.
//
// Example:
//
//	loci-cli -o release -prefix "SECURE_" -encrypt -targets windows-amd64,macos-arm64 ./docs report.pdf
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/biochemajor/loci/internal/sfx"
	"github.com/biochemajor/loci/internal/stubassets"
	"golang.org/x/term"
)

func main() {
	var (
		out       = flag.String("o", "archive", "output base path (target suffix and extension are appended)")
		prefix    = flag.String("prefix", "", "prefix added to each extracted file name")
		suffix    = flag.String("suffix", "", "suffix added before each extracted file's extension")
		encrypt   = flag.Bool("encrypt", false, "encrypt the archive with a password (prompted)")
		password  = flag.String("password", "", "password for encryption (insecure; prefer -encrypt to be prompted)")
		targetCSV = flag.String("targets", defaultTargets(), "comma-separated target keys: "+targetKeys())
		list      = flag.Bool("list-targets", false, "list available targets and exit")
	)
	flag.Usage = usage
	flag.Parse()

	if *list {
		for _, t := range stubassets.Targets {
			status := "not built"
			if stubassets.Available(t) {
				status = "ready"
			}
			fmt.Printf("  %-16s %-24s [%s]\n", t.Key, t.Label, status)
		}
		return
	}

	sources := flag.Args()
	if len(sources) == 0 {
		usage()
		os.Exit(2)
	}

	targets, err := resolveTargets(*targetCSV)
	if err != nil {
		fatal(err)
	}

	pw := *password
	if *encrypt && pw == "" {
		pw, err = promptPassword()
		if err != nil {
			fatal(err)
		}
	}
	if pw != "" {
		// -password without -encrypt still encrypts.
	}

	container, res, err := sfx.BuildContainer(sfx.Options{
		Sources:  sources,
		Rename:   sfx.Rename{Prefix: *prefix, Suffix: *suffix},
		Password: pw,
	})
	if err != nil {
		fatal(err)
	}

	fmt.Printf("Archived %d file(s), %d bytes compressed, encrypted=%v\n",
		len(res.Files), res.ContainerSize, res.Encrypted)
	fmt.Printf("Archive SHA-256 (post-compression): %s\n", res.ArchiveSHA256)

	for _, t := range targets {
		stub, err := stubassets.Stub(t)
		if err != nil {
			fatal(err)
		}
		outPath := *out + "-" + t.Key + t.OutExt
		if err := sfx.WriteSFX(stub, container, outPath, t.UnixExec); err != nil {
			fatal(err)
		}
		fmt.Printf("  wrote %s (%s)\n", outPath, t.Label)
	}
}

func resolveTargets(csv string) ([]stubassets.Target, error) {
	var out []stubassets.Target
	for _, key := range strings.Split(csv, ",") {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		t, ok := stubassets.TargetByKey(key)
		if !ok {
			return nil, fmt.Errorf("unknown target %q (valid: %s)", key, targetKeys())
		}
		out = append(out, t)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no targets selected")
	}
	return out, nil
}

func defaultTargets() string {
	// Default to the platforms the task targets.
	return "windows-amd64,macos-arm64,macos-amd64"
}

func targetKeys() string {
	keys := make([]string, 0, len(stubassets.Targets))
	for _, t := range stubassets.Targets {
		keys = append(keys, t.Key)
	}
	return strings.Join(keys, ", ")
}

func promptPassword() (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", fmt.Errorf("cannot prompt for password: stdin is not a terminal (use -password)")
	}
	fmt.Print("Password: ")
	a, err := term.ReadPassword(fd)
	fmt.Println()
	if err != nil {
		return "", err
	}
	fmt.Print("Confirm:  ")
	b, err := term.ReadPassword(fd)
	fmt.Println()
	if err != nil {
		return "", err
	}
	if string(a) != string(b) {
		return "", fmt.Errorf("passwords do not match")
	}
	if len(a) == 0 {
		return "", fmt.Errorf("empty password")
	}
	return string(a), nil
}

func usage() {
	fmt.Fprintf(os.Stderr, `loci-cli — build self-extracting archives

Usage:
  loci-cli [flags] <file-or-dir> [more...]

Flags:
`)
	flag.PrintDefaults()
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
