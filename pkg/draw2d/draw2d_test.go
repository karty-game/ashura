package draw2d

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/spritepaint"
)

func fixtureImage(t *testing.T, width, height int) *Image {
	t.Helper()

	pixels := make([]byte, width*height*4)
	for index := range pixels {
		pixels[index] = 255
	}

	image, err := NewImage(width, height, pixels)
	if err != nil {
		t.Fatal(err)
	}

	return image
}

func floats(data []byte) [20]float32 {
	var values [20]float32
	for index := range values {
		values[index] = math.Float32frombits(binary.LittleEndian.Uint32(data[index*4:]))
	}

	return values
}

func panics(t *testing.T, call func()) {
	t.Helper()

	defer func() {
		if recover() == nil {
			t.Error("expected programming-error panic")
		}
	}()

	call()
}

func TestPreparedImagesAndRegions(t *testing.T) {
	t.Parallel()

	pixels := []byte{255, 0, 0, 255, 0, 255, 0, 128, 0, 0, 255, 0, 255, 255, 255, 255}

	image, err := NewImage(2, 2, pixels)
	if err != nil {
		t.Fatal(err)
	}

	pixels[0] = 0

	if image.page.Pixels[0] != 255 {
		t.Fatal("admitted image aliases caller memory")
	}

	region, err := image.Region(1, 0, 1, 2)
	if err != nil {
		t.Fatal(err)
	}

	nested, err := region.Region(0, 1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}

	if nested.page != image.page || nested.x != 1 || nested.y != 1 || nested.Width() != 1 || nested.Height() != 1 {
		t.Fatal("region did not retain page-relative bounds")
	}

	for _, bounds := range [][4]int{{-1, 0, 1, 1}, {0, 0, 0, 1}, {1, 1, 2, 1}, {math.MaxInt, 0, 1, 1}} {
		if _, err := image.Region(bounds[0], bounds[1], bounds[2], bounds[3]); err == nil {
			t.Errorf("invalid region admitted: %v", bounds)
		}
	}

	if _, err := NewImage(math.MaxInt, 2, nil); err == nil {
		t.Fatal("unbounded image admitted")
	}
}

func TestDefaultsPixelBoundsAndFullTransform(t *testing.T) {
	t.Parallel()

	screen := NewScreen()
	sprite := screen.NewSprite(fixtureImage(t, 32, 32)).SetPosition(10, 10)
	frame := screen.Frame(100, 80)

	values := floats(frame.Data)
	if values[0] != 1 || values[3] != 1 || values[4] != 10 || values[5] != 10 || values[6] != 32 || values[7] != 32 ||
		values[4]+values[6] != 42 ||
		values[5]+values[7] != 42 {
		t.Fatalf("default pixel edges: %v", values)
	}

	if values[12] != 1 || values[13] != 1 || values[14] != 1 || values[15] != 1 {
		t.Fatal("nonidentity appearance defaults")
	}

	sprite.SetPivot(16, 16).SetScale(-2, 3).SetRotation(math.Pi / 2)

	values = floats(screen.Frame(100, 80).Data)
	// The source pivot still lands on (10,10) after scale and rotation.
	x := values[0]*16 + values[2]*16 + values[4]

	y := values[1]*16 + values[3]*16 + values[5]
	if math.Abs(float64(x-10)) > 0.0001 || math.Abs(float64(y-10)) > 0.0001 || math.Abs(float64(values[1]+2)) > 0.0001 ||
		math.Abs(float64(values[2]+3)) > 0.0001 {
		t.Fatalf("pivot/rotation/negative scale: %v", values)
	}

	width, height := screen.Size()

	convertedX, convertedY := screen.FromLogical(10, 10, 50, 40)
	if width != 100 || height != 80 || convertedX != 20 || convertedY != 20 {
		t.Fatal("framebuffer/logical conversion failed")
	}

	screen.Resize(200, 160)

	convertedX, convertedY = screen.FromLogical(10, 10, 50, 40)
	if convertedX != 40 || convertedY != 40 || sprite.x != 10 || sprite.y != 10 {
		t.Fatal("resize moved sprite or delayed coordinate conversion")
	}
}

func TestOrderBatchingAndSharedPageLifetime(t *testing.T) {
	t.Parallel()

	screen := NewScreen()
	image := fixtureImage(t, 8, 8)
	left, _ := image.Region(0, 0, 4, 8)
	right, _ := image.Region(4, 0, 4, 8)
	first := screen.NewSprite(left)
	second := screen.NewSprite(right)
	third := screen.NewSprite(left)

	frame := screen.Frame(80, 80)
	if len(frame.Textures) != 1 || len(frame.Batches) != 1 || frame.Batches[0].Count != 3 {
		t.Fatal("shared regions did not batch")
	}

	third.SetOrder(-1)

	frame = screen.Frame(80, 80)
	if len(frame.Batches) != 2 || frame.Batches[0].First != 2 || frame.Batches[1].First != 0 || frame.Batches[1].Count != 2 {
		t.Fatalf("painter order changed for batching: %+v", frame.Batches)
	}

	first.SetImage(right)

	frame = screen.Frame(80, 80)
	if frame.Batches[0].First != 2 || len(frame.Textures) != 1 {
		t.Fatal("image update changed stable order or duplicated region page")
	}

	first.SetVisible(false)
	second.SetOpacity(0)
	third.SetScale(0, 1)

	frame = screen.Frame(80, 80)
	if len(frame.Batches) != 0 || len(frame.Textures) != 1 || screen.Stats().ImageBytes != 8*8*4 {
		t.Fatal("hidden sprites lost shared residency")
	}

	first.Destroy()
	second.Destroy()
	third.Destroy()

	frame = screen.Frame(80, 80)
	if len(frame.Textures) != 0 || len(frame.Batches) != 0 || screen.Stats().ImageBytes != 0 {
		t.Fatal("last owner did not release page")
	}
}

func TestClipOpacityAndAtomicInvalidMutations(t *testing.T) {
	t.Parallel()

	screen := NewScreen().SetClip(5, 5, 20, 20)
	sprite := screen.NewSprite(fixtureImage(t, 32, 32)).SetClip(10, 0, 30, 12).SetTint(2, -1, 0.5).SetOpacity(2)

	frame := screen.Frame(50, 50)
	if frame.Batches[0].Clip != [4]uint32{10, 5, 15, 7} {
		t.Fatalf("clip intersection: %v", frame.Batches[0].Clip)
	}

	values := floats(frame.Data)
	if values[12] != 1 || values[13] != 0 || values[14] != 0.5 || values[15] != 1 {
		t.Fatal("color clamp failed")
	}

	before := append([]byte(nil), frame.Data...)

	panics(t, func() { sprite.SetTint(0, 0, float32(math.NaN())) })
	panics(t, func() { sprite.SetPosition(1, float32(math.Inf(1))) })
	panics(t, func() { sprite.SetClip(0, 0, 1, -1) })
	panics(t, func() { sprite.SetImage(nil) })
	panics(t, func() { screen.SetClip(math.MaxInt32, 0, 1, 1) })

	if !bytes.Equal(before, screen.Frame(50, 50).Data) {
		t.Fatal("invalid setter partially changed state")
	}

	sprite.SetRotation(0.5)

	if screen.Frame(50, 50).Batches[0].Clip != [4]uint32{10, 5, 15, 7} {
		t.Fatal("rotation transformed screen clip")
	}

	sprite.SetClip(100, 100, 10, 10)

	if len(screen.Frame(50, 50).Batches) != 0 {
		t.Fatal("disjoint clip emitted draw")
	}

	sprite.ClearClip()
	screen.ClearClip()

	if screen.Frame(50, 50).Batches[0].Clip != [4]uint32{0, 0, 50, 50} {
		t.Fatal("cleared clip retained restriction")
	}

	sprite.SetClip(0, 0, 0, 10)

	if len(screen.Frame(50, 50).Batches) != 0 {
		t.Fatal("empty clip emitted draw")
	}
}

//nolint:paralleltest // Go allocation measurement changes global scheduler state and must remain serial.
func TestRetainedDirtyRecordsSkippedRevisionsAndAllocations(t *testing.T) {
	screen := NewScreen()
	image := fixtureImage(t, 1, 1)

	sprites := make([]*Sprite, 32)
	for index := range sprites {
		sprites[index] = screen.NewSprite(image)
	}

	initial := screen.Frame(100, 100)
	before := append([]byte(nil), initial.Data...)
	revision, batchRevision := initial.Revision, initial.BatchesRevision
	stats := screen.Stats()

	if allocs := testing.AllocsPerRun(100, func() { screen.Frame(100, 100) }); allocs != 0 {
		t.Fatalf("idle frame allocations: %v", allocs)
	}

	if screen.Stats() != stats {
		t.Fatal("idle frame prepared retained sprites")
	}

	sprites[7].SetPosition(3, 4)

	frame := screen.Frame(100, 100)
	if frame.Base != revision || frame.BatchesRevision != batchRevision || len(frame.Changes) != 1 ||
		frame.Changes[0] != (spritepaint.Range{Offset: 7 * spritepaint.Stride, Size: spritepaint.Stride}) {
		t.Fatalf("isolated dirty metadata: %+v", frame)
	}

	if !bytes.Equal(before[:7*spritepaint.Stride], frame.Data[:7*spritepaint.Stride]) ||
		!bytes.Equal(before[8*spritepaint.Stride:], frame.Data[8*spritepaint.Stride:]) {
		t.Fatal("moving sprite rewrote neighbors")
	}

	if screen.Stats().PackedRecords != stats.PackedRecords+1 || screen.Stats().BatchBuilds != stats.BatchBuilds {
		t.Fatal("moving sprite rebuilt unrelated metadata")
	}

	sprites[12].SetOpacity(0.5)
	screen.Frame(100, 100)
	sprites[20].SetTint(0.5, 1, 1)

	frame = screen.Frame(100, 100)
	if frame.Base == revision || len(frame.Data) != 32*spritepaint.Stride || floats(frame.Data[7*spritepaint.Stride:])[4] != 3 ||
		floats(frame.Data[12*spritepaint.Stride:])[15] != 0.5 {
		t.Fatal("complete snapshot lost skipped revisions")
	}

	if allocs := testing.AllocsPerRun(100, func() { sprites[7].SetPosition(sprites[7].x+1, 4); screen.Frame(100, 100) }); allocs != 0 {
		t.Fatalf("warm isolated movement allocations: %v", allocs)
	}
}

func TestDestroyReuseOwnershipAndBounds(t *testing.T) {
	t.Parallel()

	screen := NewScreen()
	image := fixtureImage(t, 1, 1)
	old := screen.NewSprite(image)
	old.Destroy()
	old.Destroy()

	fresh := screen.NewSprite(image).SetPosition(4, 5)

	frame := screen.Frame(20, 20)
	if len(frame.Data) != spritepaint.Stride || len(frame.Batches) != 1 || floats(frame.Data)[4] != 4 {
		t.Fatal("destroyed slot was not safely reused")
	}

	panics(t, func() { old.SetPosition(99, 99) })

	if fresh.x != 4 {
		t.Fatal("stale handle mutated replacement")
	}

	for index := 1; index < MaxPages; index++ {
		screen.NewSprite(fixtureImage(t, 1, 1))
	}

	stats := screen.Stats()

	panics(t, func() { screen.NewSprite(fixtureImage(t, 1, 1)) })

	if screen.Stats() != stats {
		t.Fatal("page cap mutated residency before panic")
	}

	screen.Close()
	screen.Close()
	panics(t, func() { fresh.SetOpacity(1) })
	panics(t, func() { screen.NewSprite(image) })

	if screen.Stats().Sprites != 0 || screen.Stats().Pages != 0 {
		t.Fatal("close retained owners")
	}
}

func TestSpriteCountCapAndImmediateReuse(t *testing.T) {
	t.Parallel()

	screen := NewScreen()
	image := fixtureImage(t, 1, 1)

	var first *Sprite

	for index := range MaxSprites {
		sprite := screen.NewSprite(image)
		if index == 0 {
			first = sprite
		}
	}

	panics(t, func() { screen.NewSprite(image) })
	first.Destroy()
	screen.NewSprite(image)

	if screen.Stats().Sprites != MaxSprites || len(screen.slots) != MaxSprites {
		t.Fatal("count cap or immediate slot reuse failed")
	}
}

func TestTransformBoundsAreAtomicAndDerivedRecordsFinite(t *testing.T) {
	t.Parallel()

	screen := NewScreen()
	sprite := screen.NewSprite(fixtureImage(t, 1, 1)).SetPosition(10, 20).SetScale(2, 3).SetPivot(4, 5)
	before := append([]byte(nil), screen.Frame(50, 50).Data...)

	for _, call := range []func(){
		func() { sprite.SetPosition(12, math.MaxFloat32) },
		func() { sprite.SetScale(math.MaxFloat32, 4) },
		func() { sprite.SetPivot(4, -math.MaxFloat32) },
	} {
		panics(t, call)
	}

	if !bytes.Equal(before, screen.Frame(50, 50).Data) {
		t.Fatal("out-of-range transform partially mutated record")
	}

	sprite.SetPosition(MaxTransformValue, -MaxTransformValue).
		SetScale(-MaxTransformValue, MaxTransformValue).
		SetPivot(MaxTransformValue, -MaxTransformValue).
		SetRotation(math.MaxFloat32)

	for _, value := range floats(screen.Frame(50, 50).Data) {
		if math.IsInf(float64(value), 0) || math.IsNaN(float64(value)) {
			t.Fatal("bounded transform produced nonfinite record")
		}
	}
}

func TestRetiredPagePointersAreCleared(t *testing.T) {
	t.Parallel()

	screen := NewScreen()
	sprite := screen.NewSprite(fixtureImage(t, 1, 1))
	screen.Frame(20, 20)
	sprite.Destroy()

	frame := screen.Frame(20, 20)
	for _, page := range frame.Textures[:cap(frame.Textures)] {
		if page != nil {
			t.Fatal("retired page retained in texture backing storage")
		}
	}

	for _, batch := range frame.Batches[:cap(frame.Batches)] {
		if batch.Texture != nil {
			t.Fatal("retired page retained in batch backing storage")
		}
	}

	for _, sprite := range screen.ordered[:cap(screen.ordered)] {
		if sprite != nil {
			t.Fatal("removed sprite retained in order backing storage")
		}
	}
}

func TestCopiedSpriteCannotMutateOrDestroyOriginal(t *testing.T) {
	t.Parallel()

	screen := NewScreen()
	sprite := screen.NewSprite(fixtureImage(t, 1, 1)).SetPosition(10, 20)
	screen.Frame(50, 50)
	before := screen.Stats()
	copied := *sprite

	panics(t, func() { copied.SetPosition(99, 99) })
	panics(t, copied.Destroy)

	if sprite.destroyed || screen.slots[sprite.slot] != sprite || sprite.x != 10 || sprite.y != 20 || screen.Stats() != before {
		t.Fatal("copied sprite altered the original handle, ownership or residency")
	}

	sprite.SetPosition(11, 21)

	frame := screen.Frame(50, 50)
	if len(frame.Batches) != 1 || len(frame.Textures) != 1 || floats(frame.Data)[4] != 11 {
		t.Fatal("original sprite stopped rendering after copied-handle rejection")
	}

	sprite.Destroy()

	frame = screen.Frame(50, 50)
	if len(frame.Batches) != 0 || len(frame.Textures) != 0 || screen.Stats().Sprites != 0 || screen.Stats().ImageBytes != 0 {
		t.Fatal("original sprite could not release ownership normally")
	}
}

func TestSamePageRegionChangePreservesSubmission(t *testing.T) {
	t.Parallel()

	screen := NewScreen()
	image := fixtureImage(t, 4, 2)

	left, err := image.Region(0, 0, 2, 2)
	if err != nil {
		t.Fatal(err)
	}

	right, err := image.Region(2, 0, 2, 2)
	if err != nil {
		t.Fatal(err)
	}

	sprite := screen.NewSprite(left)
	screen.NewSprite(left)
	frame := screen.Frame(50, 50)
	neighbor := append([]byte(nil), frame.Data[spritepaint.Stride:]...)
	stats := screen.Stats()
	batchRevision := frame.BatchesRevision

	sprite.SetImage(right)

	frame = screen.Frame(50, 50)
	if screen.Stats().BatchBuilds != stats.BatchBuilds || frame.BatchesRevision != batchRevision {
		t.Fatal("same-page region change rebuilt submission metadata")
	}

	if len(frame.Changes) != 1 || frame.Changes[0] != (spritepaint.Range{Offset: 0, Size: spritepaint.Stride}) ||
		screen.Stats().PackedRecords != stats.PackedRecords+1 {
		t.Fatal("region change did not isolate its 80-byte record")
	}

	if !bytes.Equal(neighbor, frame.Data[spritepaint.Stride:]) || floats(frame.Data)[8] != 0.5 || len(frame.Textures) != 1 {
		t.Fatal("region switch changed neighboring records, UVs or page residency")
	}
}
