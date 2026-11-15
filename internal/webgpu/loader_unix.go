//go:build linux || darwin

package wgpu

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
	"github.com/go-webgpu/goffi/types"
)

// unixLibrary wraps goffi library handle to implement the Library interface.
type unixLibrary struct {
	handle unsafe.Pointer
	name   string
}

// unixProc wraps a goffi function pointer and prepared CIF.
type unixProc struct {
	lib            *unixLibrary
	name           string
	fnPtr          unsafe.Pointer
	cif            types.CallInterface
	cifMu          sync.Mutex
	prepared       bool
	aggregateIndex int
	aggregateType  *types.TypeDescriptor
}

// loadLibrary loads a shared library using goffi.LoadLibrary.
// Returns a Library interface and an error if the library cannot be found.
func loadLibrary(name string) (Library, error) {
	handle, err := ffi.LoadLibrary(name)
	if err != nil {
		return nil, fmt.Errorf("dlopen %s: %w", name, err)
	}

	return &unixLibrary{
		handle: handle,
		name:   name,
	}, nil
}

// NewProc retrieves a procedure from the Unix shared library.
func (u *unixLibrary) NewProc(name string) Proc {
	if u.handle == nil {
		// Return a proc that will fail on Call
		return &unixProc{
			lib:      u,
			name:     name,
			fnPtr:    nil,
			prepared: false,
		}
	}

	fnPtr, err := ffi.GetSymbol(u.handle, name)
	if err != nil {
		// Return a proc that will fail on Call
		return &unixProc{
			lib:      u,
			name:     name,
			fnPtr:    nil,
			prepared: false,
		}
	}

	index, aggregate := aggregateArgument(name)
	return &unixProc{lib: u, name: name, fnPtr: fnPtr, aggregateIndex: index, aggregateType: aggregate}

}

// Call invokes the Unix procedure with the given arguments.
// This uses goffi's CallFunction with lazy CIF preparation.
//
// Note: WebGPU functions have varying signatures, so we use a conservative
// approach: prepare CIF on first call with actual argument count.
// Most WebGPU functions return uintptr (handles) or void.
func (u *unixProc) Call(args ...uintptr) (uintptr, uintptr, error) {
	if u.fnPtr == nil {
		return 0, 0, fmt.Errorf("wgpu: failed to get symbol %s from %s", u.name, u.lib.name)
	}

	// Lazy CIF preparation on first call
	u.cifMu.Lock()
	if !u.prepared {
		argCount := len(args)
		argTypes := make([]*types.TypeDescriptor, argCount)
		for i := 0; i < argCount; i++ {
			argTypes[i] = types.PointerTypeDescriptor // Conservative: treat all args as uintptr
		}

		if u.aggregateType != nil {
			argTypes[u.aggregateIndex] = u.aggregateType
		}

		// Use platform-specific calling convention
		// Linux/macOS use System V AMD64 ABI (UnixCallingConvention)
		err := ffi.PrepareCallInterface(
			&u.cif,
			types.UnixCallingConvention,
			types.PointerTypeDescriptor, // Most WebGPU functions return uintptr handle
			argTypes,
		)
		if err != nil {
			u.cifMu.Unlock()
			return 0, 0, fmt.Errorf("wgpu: failed to prepare CIF for %s: %w", u.name, err)
		}
		u.prepared = true
	}
	u.cifMu.Unlock()

	// Prepare argument pointers
	argPtrs := make([]unsafe.Pointer, len(args))
	for i := range args {
		if u.aggregateType != nil && u.aggregateIndex == i {
			argPtrs[i] = ptrFromUintptr(args[i])
		} else {
			argPtrs[i] = unsafe.Pointer(&args[i])
		}
	}

	// Call the function
	var result uintptr
	_, err := ffi.CallFunction(&u.cif, u.fnPtr, unsafe.Pointer(&result), argPtrs)
	if err != nil {
		return 0, 0, fmt.Errorf("wgpu: call to %s failed: %w", u.name, err)
	}

	// Return result, 0 (no secondary return), nil error
	// This matches Windows syscall.LazyProc.Call signature
	return result, 0, nil
}

// CallFloat32 invokes a procedure whose native return type is float32.
//
// Proc.Call uses a pointer-sized return descriptor for the rest of the API.
// A float32 is returned in the platform floating-point register instead, so
// it needs a call interface prepared with FloatTypeDescriptor.
func (u *unixProc) CallFloat32(args ...uintptr) (float32, error) {
	if u.fnPtr == nil {
		return 0, fmt.Errorf("wgpu: failed to get symbol %s from %s", u.name, u.lib.name)
	}
	return callFloat32(nativeFloat32CallOps, u.name, types.UnixCallingConvention, u.fnPtr, args...)
}

func aggregateArgument(name string) (int, *types.TypeDescriptor) {
	callback := &types.TypeDescriptor{Kind: types.StructType, Size: 40, Alignment: 8, Members: []*types.TypeDescriptor{
		types.PointerTypeDescriptor, types.UInt32TypeDescriptor, types.PointerTypeDescriptor, types.PointerTypeDescriptor, types.PointerTypeDescriptor}}
	stringView := &types.TypeDescriptor{Kind: types.StructType, Size: 16, Alignment: 8, Members: []*types.TypeDescriptor{types.PointerTypeDescriptor, types.UInt64TypeDescriptor}}
	switch name {
	case "wgpuInstanceRequestAdapter", "wgpuAdapterRequestDevice":
		return 2, callback
	case "wgpuBufferMapAsync":
		return 4, callback
	case "wgpuDevicePopErrorScope", "wgpuQueueOnSubmittedWorkDone", "wgpuShaderModuleGetCompilationInfo":
		return 1, callback
	case "wgpuAdapterInfoFreeMembers":
		return 0, &types.TypeDescriptor{Kind: types.StructType, Size: unsafe.Sizeof(AdapterInfo{}), Alignment: 8, Members: []*types.TypeDescriptor{types.PointerTypeDescriptor, stringView, stringView, stringView, stringView, types.UInt32TypeDescriptor, types.UInt32TypeDescriptor, types.UInt32TypeDescriptor, types.UInt32TypeDescriptor, types.UInt32TypeDescriptor, types.UInt32TypeDescriptor}}
	case "wgpuSupportedFeaturesFreeMembers":
		return 0, &types.TypeDescriptor{Kind: types.StructType, Size: 16, Alignment: 8, Members: []*types.TypeDescriptor{types.UInt64TypeDescriptor, types.PointerTypeDescriptor}}
	}
	return 0, nil
}
