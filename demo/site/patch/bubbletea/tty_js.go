//go:build js && wasm

// Added to Bubble Tea for the demo on the site, which has no terminal but
// the page's: there is no raw mode to enter, the page tells the size, and
// nothing can be suspended. Bubble Tea has no files for js otherwise.

package tea

func (p *Program) initInput() error { return nil }

func (p *Program) listenForResize(done chan struct{}) { close(done) }

const suspendSupported = false

func suspendProcess() {}
