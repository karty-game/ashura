package resource

import (
	"context"
	"fmt"

	"github.com/karty-game/ashura/internal/resourcedata"
)

// Slice identifies an entry at a prepared byte offset within a resource.
type Slice struct {
	Asset  Asset
	Offset int
}

// ReadVerifiedSlices verifies the parent and every requested entry before
// returning immutable views in input order. Entries must be ordered, disjoint
// and kept immutable for the read. Like ReadVerified, call it from an I/O goroutine.
func ReadVerifiedSlices(ctx context.Context, base string, parent Asset, entries []Slice) (*Payload, []*Payload, error) {
	end := 0
	for _, entry := range entries {
		if !validAsset(entry.Asset) || entry.Offset < end || !validSliceRange(entry.Offset, entry.Asset.Size, parent.Size) {
			return nil, nil, fmt.Errorf("resource %q slice %s descriptor: %w", parent.ID, entry.Asset.Path, ErrInvalid)
		}

		end = entry.Offset + entry.Asset.Size
	}

	data, err := readAsset(ctx, base, parent, entries)
	if err != nil {
		return nil, nil, err
	}

	result := make([]*Payload, len(entries))
	for index, entry := range entries {
		end := entry.Offset + entry.Asset.Size
		result[index] = ownVerified(data[entry.Offset:end:end], entry.Asset.SHA256)
	}

	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	return ownVerified(data, parent.SHA256), result, nil
}

func validSliceRange(offset, size, length int) bool {
	return length >= size && length <= maxBytes && offset >= 0 && offset <= length-size
}

// SliceVerified verifies a bounded entry within an immutable verified payload.
// It retains a view of the parent's bytes, without an additional Go payload copy.
// The entry keeps the parent's backing buffer alive while the view is retained.
// Call from an I/O goroutine: browser SHA verification uses asynchronous WebCrypto.
func SliceVerified(ctx context.Context, parent *Payload, offset int64, asset Asset) (*Payload, error) {
	if ctx == nil || !resourcedata.Valid(parent) || !validAsset(asset) {
		return nil, fmt.Errorf("resource %q slice descriptor: %w", asset.ID, ErrInvalid)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	data := resourcedata.Bytes(parent)
	if len(data) > maxBytes || offset < 0 || offset > int64(len(data))-int64(asset.Size) {
		return nil, fmt.Errorf("resource %s slice bounds: %w", asset.ID, ErrInvalid)
	}

	start, end := int(offset), int(offset)+asset.Size
	entry := data[start:end:end]

	if err := verifySliceChecksum(ctx, entry, asset.SHA256); err != nil {
		if canceled := ctx.Err(); canceled != nil {
			return nil, canceled
		}

		return nil, fmt.Errorf("resource %s slice %s: %w", asset.ID, asset.Path, err)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return ownVerified(entry, asset.SHA256), nil
}
