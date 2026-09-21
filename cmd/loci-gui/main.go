// Command loci-gui is the macOS desktop builder for loci self-extracting
// archives. Pick files/folders, set an optional prefix and suffix, optionally
// encrypt with a password, choose target platforms, and Build.
//
// Build on a Mac with:
//
//	./build/build-stubs.sh          # generate the embedded extractor stubs
//	go run ./cmd/loci-gui           # or: fyne package -os darwin -icon icon.png
package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/biochemajor/loci/internal/sfx"
	"github.com/biochemajor/loci/internal/stubassets"
)

type state struct {
	win     fyne.Window
	sources []string
	sel     int
}

func main() {
	a := app.New()
	w := a.NewWindow("loci — Self-Extracting Archive Builder")
	s := &state{win: w, sel: -1}

	// --- Source list ---
	list := widget.NewList(
		func() int { return len(s.sources) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			o.(*widget.Label).SetText(s.sources[i])
		},
	)
	list.OnSelected = func(id widget.ListItemID) { s.sel = id }
	list.OnUnselected = func(id widget.ListItemID) { s.sel = -1 }

	addFile := widget.NewButton("Add File…", func() {
		dialog.ShowFileOpen(func(r fyne.URIReadCloser, err error) {
			if err != nil || r == nil {
				return
			}
			defer r.Close()
			s.sources = append(s.sources, r.URI().Path())
			list.Refresh()
		}, w)
	})
	addFolder := widget.NewButton("Add Folder…", func() {
		dialog.ShowFolderOpen(func(u fyne.ListableURI, err error) {
			if err != nil || u == nil {
				return
			}
			s.sources = append(s.sources, u.Path())
			list.Refresh()
		}, w)
	})
	removeBtn := widget.NewButton("Remove", func() {
		if s.sel < 0 || s.sel >= len(s.sources) {
			return
		}
		s.sources = append(s.sources[:s.sel], s.sources[s.sel+1:]...)
		s.sel = -1
		list.UnselectAll()
		list.Refresh()
	})
	clearBtn := widget.NewButton("Clear", func() {
		s.sources = nil
		s.sel = -1
		list.UnselectAll()
		list.Refresh()
	})
	sourceButtons := container.NewHBox(addFile, addFolder, removeBtn, clearBtn)

	// --- Rename ---
	prefixEntry := widget.NewEntry()
	prefixEntry.SetPlaceHolder("e.g. SECURE_")
	suffixEntry := widget.NewEntry()
	suffixEntry.SetPlaceHolder("e.g. _v2 (inserted before extension)")

	// --- Encryption ---
	passwordEntry := widget.NewPasswordEntry()
	passwordEntry.SetPlaceHolder("password")
	passwordEntry.Disable()
	confirmEntry := widget.NewPasswordEntry()
	confirmEntry.SetPlaceHolder("confirm password")
	confirmEntry.Disable()
	encryptCheck := widget.NewCheck("Encrypt with a password (AES-256-GCM)", func(on bool) {
		if on {
			passwordEntry.Enable()
			confirmEntry.Enable()
		} else {
			passwordEntry.Disable()
			confirmEntry.Disable()
		}
	})

	// --- Targets ---
	targetChecks := map[string]*widget.Check{}
	var targetBoxes []fyne.CanvasObject
	defaultOn := map[string]bool{"windows-amd64": true, "macos-arm64": true, "macos-amd64": true}
	for _, t := range stubassets.Targets {
		t := t
		label := t.Label
		chk := widget.NewCheck(label, nil)
		if !stubassets.Available(t) {
			chk.Disable()
			chk.Text = label + "  (run build-stubs.sh)"
		} else if defaultOn[t.Key] {
			chk.SetChecked(true)
		}
		targetChecks[t.Key] = chk
		targetBoxes = append(targetBoxes, chk)
	}

	// --- Output ---
	outDirEntry := widget.NewEntry()
	outDirEntry.SetPlaceHolder("output folder")
	chooseDir := widget.NewButton("Choose…", func() {
		dialog.ShowFolderOpen(func(u fyne.ListableURI, err error) {
			if err != nil || u == nil {
				return
			}
			outDirEntry.SetText(u.Path())
		}, w)
	})
	baseNameEntry := widget.NewEntry()
	baseNameEntry.SetText("archive")

	status := widget.NewLabel("")
	status.Wrapping = fyne.TextWrapWord

	buildBtn := widget.NewButton("Build", func() {
		s.build(prefixEntry, suffixEntry, encryptCheck, passwordEntry, confirmEntry,
			targetChecks, outDirEntry, baseNameEntry, status)
	})
	buildBtn.Importance = widget.HighImportance

	form := container.NewVBox(
		widget.NewLabelWithStyle("Source files & folders", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		sourceButtons,
		container.NewGridWrap(fyne.NewSize(560, 180), list),
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Rename on extraction", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewForm(
			widget.NewFormItem("Prefix", prefixEntry),
			widget.NewFormItem("Suffix", suffixEntry),
		),
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Security", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		encryptCheck,
		passwordEntry,
		confirmEntry,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Target platforms", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewVBox(targetBoxes...),
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Output", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewBorder(nil, nil, nil, chooseDir, outDirEntry),
		widget.NewForm(widget.NewFormItem("Base name", baseNameEntry)),
		buildBtn,
		status,
	)

	w.SetContent(container.NewVScroll(form))
	w.Resize(fyne.NewSize(600, 780))
	w.ShowAndRun()
}

func (s *state) build(prefixEntry, suffixEntry *widget.Entry, encryptCheck *widget.Check,
	passwordEntry, confirmEntry *widget.Entry, targetChecks map[string]*widget.Check,
	outDirEntry, baseNameEntry *widget.Entry, status *widget.Label) {

	if len(s.sources) == 0 {
		dialog.ShowError(fmt.Errorf("add at least one file or folder"), s.win)
		return
	}
	outDir := strings.TrimSpace(outDirEntry.Text)
	if outDir == "" {
		dialog.ShowError(fmt.Errorf("choose an output folder"), s.win)
		return
	}
	base := strings.TrimSpace(baseNameEntry.Text)
	if base == "" {
		base = "archive"
	}

	var targets []stubassets.Target
	for _, t := range stubassets.Targets {
		if chk := targetChecks[t.Key]; chk != nil && chk.Checked {
			targets = append(targets, t)
		}
	}
	if len(targets) == 0 {
		dialog.ShowError(fmt.Errorf("select at least one target platform"), s.win)
		return
	}

	password := ""
	if encryptCheck.Checked {
		if passwordEntry.Text == "" {
			dialog.ShowError(fmt.Errorf("enter a password or turn off encryption"), s.win)
			return
		}
		if passwordEntry.Text != confirmEntry.Text {
			dialog.ShowError(fmt.Errorf("passwords do not match"), s.win)
			return
		}
		password = passwordEntry.Text
	}

	status.SetText("Building…")
	container, res, err := sfx.BuildContainer(sfx.Options{
		Sources:  append([]string(nil), s.sources...),
		Rename:   sfx.Rename{Prefix: prefixEntry.Text, Suffix: suffixEntry.Text},
		Password: password,
	})
	if err != nil {
		status.SetText("")
		dialog.ShowError(err, s.win)
		return
	}

	var written []string
	for _, t := range targets {
		stub, err := stubassets.Stub(t)
		if err != nil {
			status.SetText("")
			dialog.ShowError(err, s.win)
			return
		}
		outPath := filepath.Join(outDir, base+"-"+t.Key+t.OutExt)
		if err := sfx.WriteSFX(stub, container, outPath, t.UnixExec); err != nil {
			status.SetText("")
			dialog.ShowError(err, s.win)
			return
		}
		written = append(written, outPath)
	}

	msg := fmt.Sprintf("Archived %d file(s).\nPost-compression SHA-256:\n%s\n\nWrote:\n%s",
		len(res.Files), res.ArchiveSHA256, strings.Join(written, "\n"))
	status.SetText(fmt.Sprintf("Done — %d output(s) written.", len(written)))
	dialog.ShowInformation("Build complete", msg, s.win)
}
