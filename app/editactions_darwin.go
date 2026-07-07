//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#include <stdlib.h>
#import <Cocoa/Cocoa.h>

static void sendEditAction(const char* name) {
	SEL sel = NSSelectorFromString([NSString stringWithUTF8String:name]);
	dispatch_async(dispatch_get_main_queue(), ^{
		[NSApp sendAction:sel to:nil from:nil];
	});
}
*/
import "C"

import "unsafe"

// sendEditAction sends a standard editing selector (copy:, paste:, …) down the
// responder chain, reaching the focused WKWebView. Wails alpha's Edit menu
// roles are empty stubs on darwin — the items swallow ⌘C/⌘V and do nothing —
// so the app wires its own Edit menu to the real native actions.
func sendEditAction(selector string) {
	cs := C.CString(selector)
	defer C.free(unsafe.Pointer(cs))
	C.sendEditAction(cs)
}
