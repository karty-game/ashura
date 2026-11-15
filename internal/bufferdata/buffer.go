// Package bufferdata owns CPU staging and dirty ranges for App-owned GPU buffers.
package bufferdata

import "errors"

// Usage describes the buffer's binding role.
type Usage uint8

const (
	Uniform Usage = iota + 1
	Storage
	Alignment      uint64 = 4
	MaxUniformSize uint64 = 64 << 10
	MaxStorageSize uint64 = 128 << 20
)

// Desc specifies an explicit byte layout; no Go struct memory is uploaded implicitly.
type Desc struct {
	Label string
	Usage Usage
	Size  uint64
	Data  []byte
}

// DirtyOwner queues a buffer once when it first acquires pending writes.
type DirtyOwner interface{ MarkBufferDirty(buffer *Buffer) }

// Buffer retains CPU staging bytes. GPU storage writes are not mirrored to CPU.
// Write updates only the requested bytes and queues them for the next submission.
// Methods belong to the App's loop goroutine.
type Buffer struct {
	owner    DirtyOwner
	label    string
	usage    Usage
	bytes    []byte
	ranges   []Range
	released bool
}

// Range is one dirty interval, including its start and excluding its end.
type Range struct{ Start, End uint64 }

// Upload is an explicit dirty range awaiting a queue write.
type Upload struct {
	Buffer *Buffer
	Offset uint64
	Data   []byte
}

var (
	errDescriptor = errors.New("shader: buffer size/usage is outside portable binding limits")
	errWrite      = errors.New("shader: buffer write is unaligned or outside its bounds")
	errReleased   = errors.New("shader: buffer has been unloaded")
)

// Create validates and copies initial bytes into reusable staging storage.
func Create(owner DirtyOwner, desc Desc) (*Buffer, error) {
	size := desc.Size
	if size == 0 {
		size = uint64(len(desc.Data))
	}

	maximum := MaxStorageSize
	if desc.Usage == Uniform {
		maximum = MaxUniformSize
	}

	if desc.Usage != Uniform && desc.Usage != Storage || size == 0 || size > maximum || size%Alignment != 0 ||
		uint64(len(desc.Data)) > size ||
		uint64(len(desc.Data))%Alignment != 0 {
		return nil, errDescriptor
	}

	//nolint:gosec // Size is bounded above to 128 MiB, which fits int on all supported targets.
	buffer := &Buffer{owner: owner, label: desc.Label, usage: desc.Usage, bytes: make([]byte, int(size)), ranges: make([]Range, 0, 1)}
	if len(desc.Data) > 0 {
		if err := buffer.Write(0, desc.Data); err != nil {
			return nil, err
		}
	}

	return buffer, nil
}

// Size returns the buffer capacity in bytes.
func (buffer *Buffer) Size() uint64 { return uint64(len(buffer.bytes)) }

// Label returns the diagnostic label supplied at creation.
func (buffer *Buffer) Label() string { return buffer.label }

// Write copies bytes into a pending upload. Offset and length must be multiples
// of four. Disjoint writes stay separate so GPU-written storage gaps are preserved.
// All writes before a frame submission are visible to every dispatch in that frame.
func (buffer *Buffer) Write(offset uint64, data []byte) error {
	if buffer == nil || buffer.released {
		return errReleased
	}

	size := uint64(len(data))
	if offset%Alignment != 0 || size%Alignment != 0 || offset > buffer.Size() || size > buffer.Size()-offset {
		return errWrite
	}

	if size == 0 {
		return nil
	}

	if len(buffer.ranges) == 0 {
		buffer.owner.MarkBufferDirty(buffer)
	}

	interval := Range{Start: offset, End: offset + size}

	for index := 0; index < len(buffer.ranges); {
		existing := buffer.ranges[index]
		if interval.End < existing.Start || interval.Start > existing.End {
			index++

			continue
		}

		interval.Start = min(interval.Start, existing.Start)
		interval.End = max(interval.End, existing.End)
		last := len(buffer.ranges) - 1
		buffer.ranges[index] = buffer.ranges[last]
		buffer.ranges = buffer.ranges[:last]
	}

	buffer.ranges = append(buffer.ranges, interval)
	copy(buffer.bytes[offset:offset+size], data)

	return nil
}

// Pending exposes only written intervals to internal drivers; callers must copy
// these queue writes before the next user callback can modify staging bytes.
func Pending(buffer *Buffer, uploads []Upload) []Upload {
	for _, interval := range buffer.ranges {
		uploads = append(uploads, Upload{Buffer: buffer, Offset: interval.Start, Data: buffer.bytes[interval.Start:interval.End]})
	}

	buffer.ranges = buffer.ranges[:0]

	return uploads
}

// Kind returns the binding role.
func Kind(buffer *Buffer) Usage { return buffer.usage }

// Valid reports whether an owned handle remains live.
func Valid(buffer *Buffer) bool { return buffer != nil && !buffer.released }

// Release invalidates a handle and drops its staging memory.
func Release(buffer *Buffer) {
	buffer.released = true
	buffer.bytes = nil
	buffer.ranges = nil
	buffer.owner = nil
}
