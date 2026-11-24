//go:build !js

package lightmapbuild

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/karty-game/ashura/internal/lightmapdata"
	"github.com/karty-game/ashura/internal/lightmapkernel"
	"github.com/karty-game/ashura/pkg/lightmap"
)

const (
	// MaxRNMInfluenceLights bounds authored lights considered during host preparation.
	// A selected page still respects the unchanged 4096-light shader admission bound.
	MaxRNMInfluenceLights = 65536
	influenceCellSize     = 8
	maxInfluenceClusters  = 4096
)

var errRNMInfluence = errors.New("RNM influence selection requires prepared RNM inputs and bounded light counts")

// SelectRNMLights conservatively selects lights that can illuminate a page or
// any of its prepared diffuse patches. A light outside the chunk can illuminate
// a distant reflector and must remain in the list. Point validation precedes
// filtering, input order is preserved and the returned list is independently owned.
// This is build-only work; the shared shader and its 4096-light bound are unchanged.
// V2 ray-based inputs retain every light because their secondary hits are unknown.
func SelectRNMLights(asset *lightmap.Asset, points []lightmap.Point) ([]lightmap.Point, error) {
	if asset == nil || !asset.Document().RNM() || len(points) > MaxRNMInfluenceLights {
		return nil, errRNMInfluence
	}

	document := asset.Document()
	for index, point := range points {
		if _, _, err := lightmapkernel.Parameters(document, point); err != nil {
			return nil, fmt.Errorf("RNM influence light %d: %w", index, err)
		}
	}

	if !document.Transport() {
		if len(points) > lightmapkernel.RNMMaxPoints {
			return nil, errRNMInfluence
		}

		return slices.Clone(points), nil
	}

	clusters, bounds := pageInfluenceBounds(document)

	selected := make([]lightmap.Point, 0, min(len(points), lightmapkernel.RNMMaxPoints))
	for _, point := range points {
		if point.Intensity == 0 || point.Color == [3]float32{} || !bounds.intersects(point) {
			continue
		}

		for _, cluster := range clusters {
			if !cluster.intersects(point) {
				continue
			}

			if len(selected) == lightmapkernel.RNMMaxPoints {
				return nil, errRNMInfluence
			}

			selected = append(selected, point)

			break
		}
	}

	return selected, nil
}

type influenceBounds struct {
	minimum, maximum [3]float64
	valid            bool
}

func (bounds *influenceBounds) add(position [3]float64) {
	if !bounds.valid {
		bounds.minimum, bounds.maximum, bounds.valid = position, position, true

		return
	}

	for axis := range position {
		bounds.minimum[axis] = min(bounds.minimum[axis], position[axis])
		bounds.maximum[axis] = max(bounds.maximum[axis], position[axis])
	}
}

func (bounds *influenceBounds) intersects(point lightmap.Point) bool {
	if !bounds.valid {
		return false
	}

	var squared float64

	for axis, value := range point.Position {
		coordinate := float64(value)
		distance := max(bounds.minimum[axis]-coordinate, coordinate-bounds.maximum[axis], 0)
		squared += distance * distance
	}

	return squared <= float64(point.Range)*float64(point.Range)
}

func influencePosition(data []byte) [3]float64 {
	var position [3]float64
	for axis := range position {
		position[axis] = float64(math.Float32frombits(binary.LittleEndian.Uint32(data[axis*4:])))
	}

	return position
}

func pageInfluenceBounds(document *lightmapdata.Document) ([]influenceBounds, influenceBounds) {
	var receivers, all, overflow influenceBounds

	for offset := 0; offset < len(document.Receivers); offset += lightmapdata.ReceiverStride {
		receiver := document.Receivers[offset:]
		if math.Float32frombits(binary.LittleEndian.Uint32(receiver[12:])) <= 0 {
			continue
		}

		position := influencePosition(receiver)
		receivers.add(position)
		all.add(position)
	}

	clusters := []influenceBounds{receivers}
	cells := make(map[[3]float64]int)

	for offset := 0; offset < len(document.GIPatches); offset += lightmapdata.GIPatchStride {
		position := influencePosition(document.GIPatches[offset:])
		all.add(position)

		var cell [3]float64
		for axis := range cell {
			cell[axis] = math.Floor(position[axis] / influenceCellSize)
		}

		index, found := cells[cell]
		if !found {
			if len(cells) == maxInfluenceClusters {
				overflow.add(position)

				continue
			}

			index = len(clusters)
			cells[cell] = index

			clusters = append(clusters, influenceBounds{})
		}

		clusters[index].add(position)
	}

	if overflow.valid {
		clusters = append(clusters, overflow)
	}

	return clusters, all
}
