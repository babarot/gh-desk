//go:build js && wasm

// Added to atotto/clipboard for the demo on the site: the text boxes of
// Bubbles import it, and it has no files for js otherwise.

package clipboard

import "errors"

var errNoClipboard = errors.New("clipboard: not in the demo")

func readAll() (string, error) { return "", errNoClipboard }

func writeAll(string) error { return errNoClipboard }
