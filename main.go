package main

// This package exports functions for use as a shared library (.so)
// intended to work as a Nezha monitoring agent plugin.

import "C"

// Hello returns a greeting string exported to C consumers of the shared library.
//export Hello
func Hello() *C.char {
	return C.CString("Hello from nxso!")
}

// Version returns the current version of the shared library.
//export Version
func Version() *C.char {
	return C.CString("1.0.0")
}

// Main is required by Go when building with CGO_ENABLED=1.
func main() {}
