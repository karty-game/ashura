//go:build js && wasm

package browser

import (
	"fmt"
	"syscall/js"
)

type promiseResult struct {
	value js.Value
	err   error
}

func awaitPromise(promise js.Value) (js.Value, error) {
	result := make(chan promiseResult, 1)
	resolve := js.FuncOf(func(_ js.Value, args []js.Value) any {
		value := js.Undefined()
		if len(args) > 0 {
			value = args[0]
		}

		result <- promiseResult{value: value}

		return nil
	})
	reject := js.FuncOf(func(_ js.Value, args []js.Value) any {
		message := "promise rejected"

		if len(args) > 0 {
			if args[0].Type() == js.TypeObject && !args[0].IsNull() {
				message = args[0].Get("message").String()
			} else {
				message = args[0].String()
			}
		}

		result <- promiseResult{err: fmt.Errorf("%w: %s", errPromiseRejected, message)}

		return nil
	})
	promise.Call("then", resolve).Call("catch", reject)

	resolved := <-result

	resolve.Release()
	reject.Release()

	return resolved.value, resolved.err
}

func jsObject(values map[string]any) js.Value {
	object := js.Global().Get("Object").New()
	for key, value := range values {
		object.Set(key, value)
	}

	return object
}
