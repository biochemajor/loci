# loci — self-extracting archive builder
.PHONY: stubs cli gui test app clean

# Cross-compile the extractor stubs into the embed directory (run this first).
stubs:
	./build/build-stubs.sh

# Build the command-line builder.
cli: stubs
	mkdir -p dist
	go build -trimpath -o dist/loci-cli ./cmd/loci-cli

# Run the desktop GUI builder (needs a Mac + Xcode command line tools).
gui: stubs
	go run ./cmd/loci-gui

# Package a distributable macOS .app (needs the fyne CLI: go install fyne.io/fyne/v2/cmd/fyne@latest).
app: stubs
	fyne package -os darwin -name loci -src ./cmd/loci-gui

test:
	go test ./internal/...

clean:
	rm -rf dist internal/stubassets/stubs/loci-stub-*
