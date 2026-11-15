package debug

import (
	"fmt"
	"math"
	"strconv"
	"time"
)

const (
	rowFPS = iota
	rowInterval
	rowCPU
	rowSceneDraws
	rowSceneInstances
	rowSceneTriangles
	rowSpriteDraws
	rowSpriteInstances
	rowSpriteTriangles
	rowUIDraws
	rowUIQuads
	rowUITriangles
	rowDraws
	rowDispatches
	rowUpdate
	rowSchedule
	rowScene
	rowPrepare
	rowSurface
	rowCompute
	rowRender
	rowSubmit
	rowGPU
	rowRefresh
	rowGoHeap
	rowGoLiveHeap
	rowGoBytesFrame
	rowGoObjectsFrame
	rowGoBytesSecond
	rowJSHeap
	rowJSDelta
	rowJSAllocations
	rowLoaded
	rowResident
	rowTextureBytes
	rowCount
)

func (widgets *panelUI) render(report Report) {
	widgets.number(rowFPS, report.FPS, 1, "")
	widgets.duration(rowInterval, report.Frame.Interval)
	widgets.duration(rowCPU, report.Frame.Timings.Total)
	widgets.duration(rowUpdate, report.Frame.Timings.Update)
	widgets.duration(rowSchedule, report.Frame.Timings.Schedule)
	widgets.duration(rowScene, report.Frame.Timings.Scene)
	widgets.duration(rowPrepare, report.Frame.Timings.Prepare)
	widgets.duration(rowSurface, report.Frame.Timings.Surface)
	widgets.duration(rowCompute, report.Frame.Timings.Compute)
	widgets.duration(rowRender, report.Frame.Timings.Render)
	widgets.duration(rowSubmit, report.Frame.Timings.Submit)
	widgets.set(rowGPU, "unavailable")
	widgets.duration(rowRefresh, report.RefreshDuration)
	widgets.unsigned(rowSceneDraws, report.Frame.Scene.DrawCalls)
	widgets.unsigned(rowSceneInstances, report.Frame.Scene.Instances)
	widgets.unsigned(rowSceneTriangles, report.Frame.Scene.Triangles)
	widgets.unsigned(rowSpriteDraws, report.Frame.Sprites.DrawCalls)
	widgets.unsigned(rowSpriteInstances, report.Frame.Sprites.Instances)
	widgets.unsigned(rowSpriteTriangles, report.Frame.Sprites.Triangles)
	widgets.unsigned(rowUIDraws, report.Frame.UI.DrawCalls)
	widgets.unsigned(rowUIQuads, report.Frame.UI.Instances)
	widgets.unsigned(rowUITriangles, report.Frame.UI.Triangles)
	widgets.unsigned(rowDraws, report.Frame.DrawCalls)
	widgets.unsigned(rowDispatches, report.Frame.ComputeDispatches)

	if report.GoHeapAvailable {
		widgets.bytes(rowGoHeap, float64(report.GoHeapBytes), "")
	} else {
		widgets.set(rowGoHeap, "unavailable")
	}

	if report.GoLiveHeapAvailable {
		widgets.bytes(rowGoLiveHeap, float64(report.GoLiveHeapBytes), "")
	} else {
		widgets.set(rowGoLiveHeap, "unavailable")
	}

	if report.GoAllocationsAvailable {
		widgets.bytes(rowGoBytesFrame, report.GoBytesPerFrame, "")
		widgets.number(rowGoObjectsFrame, report.GoObjectsPerFrame, 1, "")
		widgets.bytes(rowGoBytesSecond, report.GoBytesPerSecond, "/s")
	} else {
		widgets.set(rowGoBytesFrame, "unavailable")
		widgets.set(rowGoObjectsFrame, "unavailable")
		widgets.set(rowGoBytesSecond, "unavailable")
	}

	if report.JSHeapAvailable {
		widgets.bytes(rowJSHeap, float64(report.JSHeapBytes), "")
	} else {
		widgets.set(rowJSHeap, "unavailable")
	}

	if report.JSHeapDeltaAvailable {
		widgets.bytes(rowJSDelta, report.JSHeapDeltaPerSecond, "/s")
	} else {
		widgets.set(rowJSDelta, "unavailable")
	}

	widgets.set(rowJSAllocations, "unavailable")
	widgets.signed(rowLoaded, int64(report.Frame.LoadedTextures))
	widgets.signed(rowResident, int64(report.Frame.ResidentTextures))
	widgets.bytes(rowTextureBytes, float64(report.Frame.TextureBytes), "")
}

func (widgets *panelUI) set(row int, text string) {
	if widgets.text[row] == text {
		return
	}

	if err := widgets.rows[row].SetText(text); err != nil {
		if widgets.failure == nil {
			widgets.failure = fmt.Errorf("debug metric %d: %w", row, err)
		}

		return
	}

	widgets.text[row] = text
}

func (widgets *panelUI) setBytes(row int, text []byte) {
	if sameText(text, widgets.text[row]) {
		return
	}

	widgets.set(row, string(text))
}

func sameText(value []byte, text string) bool {
	if len(value) != len(text) {
		return false
	}

	for index, character := range value {
		if character != text[index] {
			return false
		}
	}

	return true
}

func (widgets *panelUI) number(row int, value float64, precision int, suffix string) {
	text := strconv.AppendFloat(widgets.scratch[:0], value, 'f', precision, 64)
	text = append(text, suffix...)
	widgets.setBytes(row, text)
}

func (widgets *panelUI) unsigned(row int, value uint64) {
	widgets.setBytes(row, strconv.AppendUint(widgets.scratch[:0], value, 10))
}

func (widgets *panelUI) signed(row int, value int64) {
	widgets.setBytes(row, strconv.AppendInt(widgets.scratch[:0], value, 10))
}

func (widgets *panelUI) duration(row int, value time.Duration) {
	widgets.number(row, float64(value)/float64(time.Millisecond), 2, " ms")
}

func (widgets *panelUI) bytes(row int, value float64, suffix string) {
	const unit = 1024

	precision, label := 0, " B"

	if math.Abs(value) >= unit*unit {
		value /= unit * unit
		precision, label = 2, " MiB"
	} else if math.Abs(value) >= unit {
		value /= unit
		precision, label = 1, " KiB"
	}

	text := strconv.AppendFloat(widgets.scratch[:0], value, 'f', precision, 64)
	text = append(text, label...)
	text = append(text, suffix...)
	widgets.setBytes(row, text)
}
