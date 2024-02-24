package main

//#cgo LDFLAGS: -L. -lquickjs -lm
//#include "3p/quickjs.h"
//#include <stdlib.h>
//#include <string.h>
//void hostPromiseRejectionTracker(JSContext *cx, JSValue promise, JSValue reason, JS_BOOL is_handled, void *opaque);
//static JSValue throwTypeError(JSContext *cx, const char *message) { return JS_ThrowTypeError(cx, "%s", message); }
//#define Q(name) static const JSValue JS##name(void) { return JS_##name; }
//#define M(name) JSValue name(JSContext *cx, JSValue thisObj, int argc, JSValue *argv);
//Q(EXCEPTION) Q(FALSE) Q(NULL) Q(TRUE) Q(UNDEFINED) Q(UNINITIALIZED)
//M(atob) M(btoa) M(consoleLog) M(setTimeout) M(clearTimeout)
//M(request) M(read) M(close)
import "C"

import (
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"time"
	"unsafe"
)

type JSRuntime = C.JSRuntime
type JSContext = C.JSContext
type JSValue = C.JSValue

var ch chan func()
var jobs int
var timers = make(map[int]chan bool)
var timerids int
var responses = make(map[int]*http.Response)
var responseids int

func main() {
	runtime.LockOSThread() // pin goroutine to thread

	flag.Parse()

	ch = make(chan func(), 1)
	rt := C.JS_NewRuntime()
	defer C.JS_FreeRuntime(rt)
	//C.JS_SetMaxStackSize(rt, 0)
	C.JS_SetHostPromiseRejectionTracker(rt, (*C.JSHostPromiseRejectionTracker)(C.hostPromiseRejectionTracker), nil)

	cx := C.JS_NewContext(rt)
	defer C.JS_FreeContext(cx)

	global := C.JS_GetGlobalObject(cx)
	defer C.JS_FreeValue(cx, global)

	addMethod(cx, global, "request", 1, (*C.JSCFunction)(C.request))
	addMethod(cx, global, "read", 1, (*C.JSCFunction)(C.read))
	addMethod(cx, global, "close", 0, (*C.JSCFunction)(C.close))

	addMethod(cx, global, "atob", 1, (*C.JSCFunction)(C.atob))
	addMethod(cx, global, "btoa", 1, (*C.JSCFunction)(C.btoa))
	addMethod(cx, global, "setTimeout", 1, (*C.JSCFunction)(C.setTimeout))
	addMethod(cx, global, "clearTimeout", 1, (*C.JSCFunction)(C.clearTimeout))

	console := C.JS_NewObject(cx)
	addMethod(cx, console, "log", 0, (*C.JSCFunction)(C.consoleLog))
	definePropertyValue(cx, global, "console", console, C.JS_PROP_CONFIGURABLE|C.JS_PROP_ENUMERABLE)

	for _, filename := range flag.Args() {
		b, err := os.ReadFile(filename)
		if err != nil {
			panic(err)
		}
		v := eval(cx, filename, string(b), C.JS_EVAL_TYPE_GLOBAL|C.JS_EVAL_FLAG_STRICT)
		if v.isException() {
			v := C.JS_GetException(cx)
			fmt.Println("exception:", toString(cx, v))
			os.Exit(1)
		}
		defer C.JS_FreeValue(cx, v)
	}

	executePendingJobs(rt)

	for jobs > 0 {
		(<-ch)()
		jobs--
		executePendingJobs(rt)
	}
}

func executePendingJobs(rt *JSRuntime) {
	var cx *JSContext
	for 0 != C.JS_ExecutePendingJob(rt, &cx) {
	}
}

//export hostPromiseRejectionTracker
func hostPromiseRejectionTracker(cx *JSContext, promise, reason JSValue, isHandled C.JS_BOOL, opaque *C.void) {
	fmt.Println("rejected promise:", toString(cx, reason))
}

func (v JSValue) isException() bool {
	return 0 != C.JS_IsException(v)
}

func exception() JSValue {
	return C.JSEXCEPTION()
}

func undefined() JSValue {
	return C.JSUNDEFINED()
}

func eval(cx *JSContext, filename, source string, flags int) JSValue {
	f := C.CString(filename)
	defer C.free(unsafe.Pointer(f))
	s := C.CString(source)
	defer C.free(unsafe.Pointer(s))
	return C.JS_Eval(cx, s, C.strlen(s), f, C.int(flags))
}

func toInt(cx *JSContext, v JSValue) (int, bool) {
	t := C.int64_t(-1)
	ok := 0 == C.JS_ToInt64(cx, &t, v)
	return int(t), ok
}

func toString(cx *JSContext, v JSValue) string {
	p := C.JS_ToCString(cx, v)
	defer C.JS_FreeCString(cx, p)
	return C.GoString(p)
}

func fromString(cx *JSContext, s string) JSValue {
	s_ := C.CString(s)
	defer C.free(unsafe.Pointer(s_))
	return C.JS_NewString(cx, s_)
}

func throwTypeError(cx *JSContext, message string) JSValue {
	message_ := C.CString(message)
	defer C.free(unsafe.Pointer(message_))
	return C.throwTypeError(cx, message_)
}

// note: takes ownership of |val|, don't call JS_FreeValue()
func definePropertyValue(cx *JSContext, thisObj JSValue, name string, val JSValue, flags int) {
	name_ := C.CString(name)
	defer C.free(unsafe.Pointer(name_))
	if 1 != C.JS_DefinePropertyValueStr(cx, thisObj, name_, val, C.int(flags)) {
		panic("JS_DefinePropertyValue")
	}
}

func addMethod(cx *JSContext, thisObj JSValue, name string, length int, f *C.JSCFunction) {
	name_ := C.CString(name)
	defer C.free(unsafe.Pointer(name_))
	val := C.JS_NewCFunction2(cx, f, name_, C.int(length), C.JS_CFUNC_generic, 0)
	if 1 != C.JS_DefinePropertyValueStr(cx, thisObj, name_, val, C.JS_PROP_CONFIGURABLE|C.JS_PROP_WRITABLE) {
		panic("JS_DefinePropertyValue")
	}
}

//export atob
func atob(cx *JSContext, thisObj JSValue, argc C.int, argv *JSValue) JSValue {
	args := unsafe.Slice(argv, int(argc))
	if len(args) < 1 {
		return invalidCharacterError(cx)
	}
	b := []byte(toString(cx, args[0]))
	s := base64.StdEncoding.EncodeToString(b)
	return fromString(cx, s)
}

//export btoa
func btoa(cx *JSContext, thisObj JSValue, argc C.int, argv *JSValue) JSValue {
	args := unsafe.Slice(argv, int(argc))
	if len(args) < 1 {
		return invalidCharacterError(cx)
	}
	s := toString(cx, args[0])
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return invalidCharacterError(cx)
	}
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return fromString(cx, string(r))

}

func invalidCharacterError(cx *JSContext) JSValue {
	return throwTypeError(cx, "InvalidCharacterError") // FIXME throw DOMException
}

//export consoleLog
func consoleLog(cx *JSContext, thisObj JSValue, argc C.int, argv *JSValue) JSValue {
	args := unsafe.Slice(argv, int(argc))
	for i, arg := range args {
		blank := ""
		if i > 0 {
			blank = " "
		}
		fmt.Printf("%s%s", blank, toString(cx, arg))
	}
	fmt.Printf("\n")
	return undefined()
}

//export setTimeout
func setTimeout(cx *JSContext, thisObj JSValue, argc C.int, argv *JSValue) JSValue {
	args := unsafe.Slice(argv, int(argc))
	ms := 0
	if len(args) > 1 {
		var ok bool
		if ms, ok = toInt(cx, args[1]); !ok {
			return exception()
		}
	}
	timerids++
	id := timerids
	cancel := make(chan bool)
	timers[id] = cancel
	//TODO keep order of timers expiring on same tick
	args = copyValues(cx, args)
	jobs++
	go func() {
		dur := time.Duration(ms) * time.Millisecond
		cancelled := false
		select {
		case <-time.After(dur):
		case <-cancel:
			cancelled = true
		}
		ch <- func() {
			defer delete(timers, id)
			defer freeValues(cx, args)
			if cancelled {
				return
			}
			if 0 != C.JS_IsFunction(cx, args[0]) {
				global := C.JS_GetGlobalObject(cx)
				defer C.JS_FreeValue(cx, global)
				extra := args[2:]
				argc := C.int(len(extra))
				argv := unsafe.SliceData(extra)
				result := C.JS_Call(cx, args[0], global, argc, argv)
				defer C.JS_FreeValue(cx, result)
			}
			//TODO handle code strings
		}
	}()
	return C.JS_NewInt64(cx, C.int64_t(id))
}

//export clearTimeout
func clearTimeout(cx *JSContext, thisObj JSValue, argc C.int, argv *JSValue) JSValue {
	args := unsafe.Slice(argv, int(argc))
	id := 0
	if len(args) > 0 {
		var ok bool
		if id, ok = toInt(cx, args[0]); !ok {
			return exception()
		}
	}
	if cancel, ok := timers[id]; ok {
		delete(timers, id)
		cancel <- true
	}
	return undefined()
}

func copyValues(cx *JSContext, vals []JSValue) []JSValue {
	newvals := make([]JSValue, len(vals))
	for i, val := range vals {
		newvals[i] = C.JS_DupValue(cx, val)
	}
	return newvals
}

func freeValues(cx *JSContext, vals []JSValue) {
	for i, val := range vals {
		C.JS_FreeValue(cx, val)
		vals[i] = undefined()
	}
}

//export request
func request(cx *JSContext, thisObj JSValue, argc C.int, argv *JSValue) JSValue {
	args := unsafe.Slice(argv, int(argc))
	method := ""
	url := ""
	if len(args) > 1 {
		method = toString(cx, args[0])
		url = toString(cx, args[1])
	}
	if method == "" {
		return throwTypeError(cx, "bad request method")
	}
	if url == "" {
		return throwTypeError(cx, "bad URL")
	}
	resolvers := [2]JSValue{} // [resolve, reject]
	promise := C.JS_NewPromiseCapability(cx, unsafe.SliceData(resolvers[:]))
	if promise.isException() {
		return exception()
	}
	jobs++
	go func() {
		var resp *http.Response
		req, err := http.NewRequest(method, url, nil)
		if err == nil {
			resp, err = http.DefaultClient.Do(req)
		}
		ch <- func() {
			defer freeValues(cx, resolvers[:])
			f := resolvers[0]
			arg := undefined()
			if err == nil {
				responseids++
				id := responseids
				responses[id] = resp
				arg = C.JS_NewInt64(cx, C.int64_t(id))
			} else {
				f = resolvers[1]
				arg = fromString(cx, err.Error())
			}
			defer C.JS_FreeValue(cx, arg)
			result := C.JS_Call(cx, f, undefined(), 1, &arg)
			defer C.JS_FreeValue(cx, result)
		}
	}()
	return promise
}

//export read
func read(cx *JSContext, thisObj JSValue, argc C.int, argv *JSValue) JSValue {
	args := unsafe.Slice(argv, int(argc))
	id := 0
	if len(args) > 0 {
		var ok bool
		if id, ok = toInt(cx, args[0]); !ok {
			return exception()
		}
	}
	resp, ok := responses[id]
	if !ok {
		return throwTypeError(cx, "bad resource id")
	}
	resolvers := [2]JSValue{} // [resolve, reject]
	promise := C.JS_NewPromiseCapability(cx, unsafe.SliceData(resolvers[:]))
	if promise.isException() {
		return exception()
	}
	jobs++
	go func() {
		b := make([]byte, 1024)
		n, err := resp.Body.Read(b)
		ch <- func() {
			defer freeValues(cx, resolvers[:])
			f := resolvers[0]
			arg := undefined()
			if err == nil || err == io.EOF {
				arg = fromString(cx, string(b[:n])) // TODO handle multi-byte sequences
			} else {
				f = resolvers[1]
				arg = fromString(cx, err.Error())
			}
			defer C.JS_FreeValue(cx, arg)
			result := C.JS_Call(cx, f, undefined(), 1, &arg)
			defer C.JS_FreeValue(cx, result)
		}
	}()
	return promise
}

//export close
func close(cx *JSContext, thisObj JSValue, argc C.int, argv *JSValue) JSValue {
	args := unsafe.Slice(argv, int(argc))
	id := 0
	if len(args) > 0 {
		var ok bool
		if id, ok = toInt(cx, args[0]); !ok {
			return exception()
		}
	}
	if resp, ok := responses[id]; ok {
		delete(responses, id)
		resp.Body.Close()
		return undefined()
	}
	return throwTypeError(cx, "bad resource id")
}
