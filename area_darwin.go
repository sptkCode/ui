// 29 march 2014

package ui

import (
	"fmt"
	"unsafe"
	"image"
)

// #cgo LDFLAGS: -lobjc -framework Foundation -framework AppKit
// #include <stdlib.h>
//// #include <HIToolbox/Events.h>
// #include "objc_darwin.h"
// extern void areaView_drawRect(id, struct xrect);
// extern BOOL areaView_isFlipped_acceptsFirstResponder(id, SEL);
// extern void areaView_mouseMoved(id, SEL, id);
// extern void areaView_mouseDown_mouseDragged(id, SEL, id);
// extern void areaView_mouseUp(id, SEL, id);
import "C"

const (
	__goArea = "goArea"
)

var (
	_goArea C.id

	_drawRect = sel_getUid("drawRect:")
	_isFlipped = sel_getUid("isFlipped")
	_acceptsFirstResponder = sel_getUid("acceptsFirstResponder")
)

// uintptr due to a bug; see https://code.google.com/p/go/issues/detail?id=7665
type eventMethod struct {
	sel	string
	m	uintptr
}
var eventMethods = []eventMethod{
	eventMethod{"mouseMoved:", uintptr(C.areaView_mouseMoved)},
	eventMethod{"mouseDown:", uintptr(C.areaView_mouseDown_mouseDragged)},
	eventMethod{"mouseDragged:", uintptr(C.areaView_mouseDown_mouseDragged)},
	eventMethod{"mouseUp:", uintptr(C.areaView_mouseUp)},
	eventMethod{"rightMouseDown:", uintptr(C.areaView_mouseDown_mouseDragged)},
	eventMethod{"rightMouseDragged:", uintptr(C.areaView_mouseDown_mouseDragged)},
	eventMethod{"rightMouseUp:", uintptr(C.areaView_mouseUp)},
	eventMethod{"otherMouseDown:", uintptr(C.areaView_mouseDown_mouseDragged)},
	eventMethod{"otherMouseDragged:", uintptr(C.areaView_mouseDown_mouseDragged)},
	eventMethod{"otherMouseUp:", uintptr(C.areaView_mouseUp)},
}

func mkAreaClass() error {
	areaclass, err := makeAreaClass(__goArea)
	if err != nil {
		return fmt.Errorf("error creating Area backend class: %v", err)
	}
	// addAreaViewDrawMethod() is in bleh_darwin.m
	ok := C.addAreaViewDrawMethod(areaclass)
	if ok != C.BOOL(C.YES) {
		return fmt.Errorf("error overriding Area drawRect: method; reason unknown")
	}
	// TODO rename this function (it overrides anyway)
	err = addDelegateMethod(areaclass, _isFlipped,
		C.areaView_isFlipped_acceptsFirstResponder, area_boolret)
	if err != nil {
		return fmt.Errorf("error overriding Area isFlipped method: %v", err)
	}
	err = addDelegateMethod(areaclass, _acceptsFirstResponder,
		C.areaView_isFlipped_acceptsFirstResponder, area_boolret)
	if err != nil {
		return fmt.Errorf("error overriding Area acceptsFirstResponder method: %v", err)
	}
	for _, m := range eventMethods {
		err = addDelegateMethod(areaclass, sel_getUid(m.sel),
			unsafe.Pointer(m.m), delegate_void)
		if err != nil {
			return fmt.Errorf("error overriding Area %s method: %v", m.sel, err)
		}
	}
	_goArea = objc_getClass(__goArea)
	return nil
}

var (
	_drawAtPoint = sel_getUid("drawAtPoint:")
)

//export areaView_drawRect
func areaView_drawRect(self C.id, rect C.struct_xrect) {
	s := getSysData(self)
	// TODO clear clip rect
	// rectangles in Cocoa are origin/size, not point0/point1; if we don't watch for this, weird things will happen when scrolling
	// TODO change names EVERYWHERE ELSE to match
	cliprect := image.Rect(int(rect.x), int(rect.y), int(rect.x + rect.width), int(rect.y + rect.height))
	max := C.objc_msgSend_stret_rect_noargs(self, _frame)
	cliprect = image.Rect(0, 0, int(max.width), int(max.height)).Intersect(cliprect)
	if cliprect.Empty() {			// no intersection; nothing to paint
		return
	}
	i := s.handler.Paint(cliprect)
	C.drawImage(
		unsafe.Pointer(&i.Pix[0]), C.int64_t(i.Rect.Dx()), C.int64_t(i.Rect.Dy()), C.int64_t(i.Stride),
		C.int64_t(cliprect.Min.X), C.int64_t(cliprect.Min.Y))
}

//export areaView_isFlipped_acceptsFirstResponder
func areaView_isFlipped_acceptsFirstResponder(self C.id, sel C.SEL) C.BOOL {
	// yes use the same function for both methods since they're just going to return YES anyway
	// isFlipped gives us a coordinate system with (0,0) at the top-left
	// acceptsFirstResponder lets us respond to events
	return C.BOOL(C.YES)
}

var (
	_modifierFlags = sel_getUid("modifierFlags")
	_buttonNumber = sel_getUid("buttonNumber")
	_clickCount = sel_getUid("clickCount")
)

func parseModifiers(e C.id) (m Modifiers) {
	const (
		_NSShiftKeyMask = 1 << 17
		_NSControlKeyMask = 1 << 18
		_NSAlternateKeyMask = 1 << 19
		_NSCommandKeyMask = 1 << 20
	)

	mods := uintptr(C.objc_msgSend_uintret_noargs(e, _modifierFlags))
	if (mods & _NSShiftKeyMask) != 0 {
		m |= Shift
	}
	if (mods & _NSControlKeyMask) != 0 {
		// TODO
	}
	if (mods & _NSAlternateKeyMask) != 0 {
		m |= Alt
	}
	if (mods & _NSCommandKeyMask) != 0 {
		m |= Ctrl
	}
	return m
}

func areaMouseEvent(self C.id, e C.id, click bool, up bool) {
	var me MouseEvent

	s := getSysData(self)
	xp := C.getTranslatedEventPoint(self, e)
	me.Pos = image.Pt(int(xp.x), int(xp.y))
	me.Modifiers = parseModifiers(e)
	which := uint(C.objc_msgSend_intret_noargs(e, _buttonNumber)) + 1
	if which == 3 {		// swap middle and right button numbers
		which = 2
	} else if which == 2 {
		which = 3
	}
	if click && up {
		me.Up = which
	} else if click {
		me.Down = which
		me.Count = uint(C.objc_msgSend_intret_noargs(e, _clickCount))
	} else {
		which = 0			// reset for Held processing below
	}
	held := C.objc_msgSend_uintret_noargs(e, _clickCount)
	if which != 1 && (held & 1) != 0 {		// button 1
		me.Held = append(me.Held, 1)
	}
	if which != 2 && (held & 4) != 0 {		// button 2; mind the swap
		me.Held = append(me.Held, 2)
	}
	if which != 3 && (held & 2) != 0 {		// button 3
		me.Held = append(me.Held, 3)
	}
	// TODO remove this part?
	held >>= 3
	for i := uint(4); held != 0; i++ {
		if which != i && (held & 1) != 0 {
			me.Held = append(me.Held, i)
		}
		held >>= 1
	}
	repaint := s.handler.Mouse(me)
	if repaint {
		C.objc_msgSend_noargs(self, _display)
	}
}

//export areaView_mouseMoved
func areaView_mouseMoved(self C.id, sel C.SEL, e C.id) {
	// TODO not triggered?
	areaMouseEvent(self, e, false, false)
}

//export areaView_mouseDown_mouseDragged
func areaView_mouseDown_mouseDragged(self C.id, sel C.SEL, e C.id) {
	areaMouseEvent(self, e, true, false)
}

//export areaView_mouseUp
func areaView_mouseUp(self C.id, sel C.SEL, e C.id) {
	areaMouseEvent(self, e, true, true)
}

// TODO combine these with the listbox functions?

func newAreaScrollView(area C.id) C.id {
	scrollview := objc_alloc(_NSScrollView)
	scrollview = objc_msgSend_rect(scrollview, _initWithFrame,
		0, 0, 100, 100)
	C.objc_msgSend_bool(scrollview, _setHasHorizontalScroller, C.BOOL(C.YES))
	C.objc_msgSend_bool(scrollview, _setHasVerticalScroller, C.BOOL(C.YES))
	C.objc_msgSend_bool(scrollview, _setAutohidesScrollers, C.BOOL(C.YES))
	C.objc_msgSend_id(scrollview, _setDocumentView, area)
	return scrollview
}

func areaInScrollView(scrollview C.id) C.id {
	return C.objc_msgSend_noargs(scrollview, _documentView)
}

func makeArea(parentWindow C.id, alternate bool) C.id {
	area := objc_alloc(_goArea)
	area = objc_msgSend_rect(area, _initWithFrame,
		0, 0, 100, 100)
	// TODO others?
	area = newAreaScrollView(area)
	addControl(parentWindow, area)
	return area
}

// TODO combine the below with the delegate stuff

var (
	_NSView = objc_getClass("NSView")
	_NSView_Class = C.Class(unsafe.Pointer(_NSView))
)

func makeAreaClass(name string) (C.Class, error) {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))

	c := C.objc_allocateClassPair(_NSView_Class, cname, 0)
	if c == C.NilClass {
		return C.NilClass, fmt.Errorf("unable to create Objective-C class %s for Area; reason unknown", name)
	}
	C.objc_registerClassPair(c)
	return c, nil
}

var (
	area_boolret = []C.char{'c', '@', ':', 0}			// BOOL (*)(id, SEL)
)
