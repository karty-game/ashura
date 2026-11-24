//go:build !js

package lightmapper

import (
	"context"
	"encoding/binary"
	"math"
	"os"
	"strings"
	"testing"
	"time"
)

const shadowPayloadRayCount = 5

type shadowPayloadShape struct {
	name     string
	source   Scene
	normal   Vec3
	triangle bool
}

type shadowPayloadRay struct {
	origin Vec3
	limit  float32
	hit    bool
}

type shadowPayloadExpected struct {
	triangle, emitter uint32
	normal            Vec3
	mesh              bool
}

//nolint:paralleltest // One native device exercises the production predicates at exact ray endpoints.
func TestHardwareGPUShadowEndpointAndCompleteHit(t *testing.T) {
	if os.Getenv("KARTY_LIGHTMAPPER_GPU_TEST") != "1" {
		t.Skip("set KARTY_LIGHTMAPPER_GPU_TEST=1 with hardware GPU access")
	}

	worker := newShadowPayloadWorker(t)
	rays := [shadowPayloadRayCount]shadowPayloadRay{
		{Vec3{}, math.Nextafter32(2, 0), false},
		{Vec3{}, 2, false},
		{Vec3{}, math.Nextafter32(2, float32(math.Inf(1))), true},
		{Vec3{}, 3, true},
		{Vec3{3, 0, 0}, 3, false},
	}

	for _, shape := range shadowPayloadShapes() {
		t.Run(shape.name, func(t *testing.T) {
			prepared := gpuFixtureScene(t, shape.source)

			scene, err := PackGPUScene(prepared)
			if err != nil {
				t.Fatal(err)
			}

			expected := shadowPayloadIdentity(t, prepared, shape)
			tile := shadowPayloadTile(rays)

			raw, err := worker.Run(t.Context(), scene, tile)
			if err != nil {
				t.Fatal("native query", err)
			}

			for rayIndex, ray := range rays {
				assertShadowPayloadRay(t, raw, rayIndex, ray, expected)
			}
		})
	}
}

func shadowPayloadShader(t *testing.T) string {
	t.Helper()

	source, err := nativeGPUShaderSource()
	if err != nil {
		t.Fatal(err)
	}

	// Keep the production predicates; replace only worker stage entry points.
	shader := string(source)

	for stage := range stageCount {
		name := nativeStageName(stage)
		shader = strings.ReplaceAll(shader, "fn "+name+"(", "fn payload_original_"+name+"(")
	}

	var output strings.Builder
	output.WriteString(shader)

	for stage := range stageCount {
		name := nativeStageName(stage)
		if name == "lm_generate" {
			output.WriteString("\n@compute @workgroup_size(64) fn lm_generate() { if lmPoolABI!=1u{return;} }\n")
		} else if name != "lm_intersect" && name != "lm_reset" {
			output.WriteString("\n@compute @workgroup_size(64) fn ")
			output.WriteString(name)
			output.WriteString("() {}\n")
		}
	}

	output.WriteString(payloadPredicateEntry)

	return output.String()
}

func newShadowPayloadWorker(t *testing.T) *nativeGPUExecutor {
	t.Helper()

	return newPayloadWorker(t, shadowPayloadShader(t))
}

func newPayloadWorker(t *testing.T, shader string) *nativeGPUExecutor {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 30*time.Second)
	t.Cleanup(cancel)

	worker, err := newNativeGPUExecutorSource(ctx, GPUWorkerOptions{MemoryBytes: 128 << 20}, shader)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = worker.Close() })
	t.Log("hardware", worker.Device().Adapter)

	return worker
}

func shadowPayloadShapes() [4]shadowPayloadShape {
	triangle := Triangle{
		ID: 1, Positions: [3]Vec3{{-1, -1, 2}, {1, -1, 2}, {0, 1, 2}},
		Normals: [3]Vec3{{0, 0, 1}, {0, 0, 1}, {0, 0, 1}},
	}
	mesh := Scene{
		Materials: []Material{{Reflectance: Vec3{.5, .5, .5}, Emission: Vec3{2, 1, .5}, DoubleSided: true}},
		Triangles: []Triangle{triangle},
	}
	rectangle := Emitter{
		ID: 1, Kind: Rectangle, Position: Vec3{0, 0, 2}, Direction: Vec3{0, 0, -1}, Tangent: Vec3{1, 0, 0},
		Intensity: Vec3{1, 1, 1}, Size: Vec2{2, 2},
	}
	disk := Emitter{
		ID: 1, Kind: Disk, Position: Vec3{0, 0, 2}, Direction: Vec3{0, 0, -1}, Tangent: Vec3{1, 0, 0},
		Intensity: Vec3{1, 1, 1}, Radius: 1,
	}
	sphere := Emitter{
		ID: 1, Kind: Sphere, Position: Vec3{0, 0, 3}, Direction: Vec3{0, 0, -1}, Tangent: Vec3{1, 0, 0},
		Intensity: Vec3{1, 1, 1}, Radius: 1,
	}

	return [4]shadowPayloadShape{
		{"emissive-mesh", mesh, Vec3{0, 0, 1}, true},
		{"rectangle", Scene{Emitters: []Emitter{rectangle}}, Vec3{0, 0, -1}, false},
		{"disk", Scene{Emitters: []Emitter{disk}}, Vec3{0, 0, -1}, false},
		{"sphere", Scene{Emitters: []Emitter{sphere}}, Vec3{0, 0, -1}, false},
	}
}

func shadowPayloadIdentity(t *testing.T, prepared *PreparedScene, shape shadowPayloadShape) shadowPayloadExpected {
	t.Helper()

	expected := shadowPayloadExpected{triangle: math.MaxUint32, normal: shape.normal, mesh: shape.triangle}
	if shape.triangle {
		expected.triangle = 0

		emitter := prepared.TriangleEmitter(0)
		if emitter < 0 {
			t.Fatal("fixture mesh was not emissive")
		}
		//nolint:gosec // G115: a nonnegative int32 emitter index is representable as uint32.
		expected.emitter = uint32(emitter)
	}

	return expected
}

func shadowPayloadTile(rays [shadowPayloadRayCount]shadowPayloadRay) GPUTile {
	data := make([]byte, 32+shadowPayloadRayCount*32)
	binary.LittleEndian.PutUint32(data, shadowPayloadRayCount)

	for rayIndex, ray := range rays {
		values := [8]float32{ray.origin[0], ray.origin[1], ray.origin[2], ray.limit, 0, 0, 1, 0}
		for component, value := range values {
			binary.LittleEndian.PutUint32(data[32+rayIndex*32+component*4:], math.Float32bits(value))
		}
	}

	return GPUTile{Data: data, Count: shadowPayloadRayCount, Options: Options{Samples: 1, Outputs: OutputRGB | OutputRNM, Seed: 29}}
}

func assertShadowPayloadRay(t *testing.T, raw []byte, rayIndex int, ray shadowPayloadRay, expected shadowPayloadExpected) {
	t.Helper()

	word := func(slot int) uint32 { return binary.LittleEndian.Uint32(raw[(rayIndex*56+slot)*4:]) }
	if word(33) != 1 {
		t.Fatal("predicate kernel did not complete", rayIndex)
	}

	if (word(0) != 0) != ray.hit {
		t.Fatalf("ray%d maximum%g boolean visibility=%d want%v", rayIndex, ray.limit, word(0), ray.hit)
	}

	for _, offset := range []int{1, 10} {
		assertShadowPayloadHit(t, word, rayIndex, offset, ray.hit, expected)
	}
}

func assertShadowPayloadHit(t *testing.T, word func(int) uint32, rayIndex, offset int, hit bool, expected shadowPayloadExpected) {
	t.Helper()

	if !hit {
		if word(offset) != math.MaxUint32 || word(offset+1) != math.MaxUint32 {
			t.Fatalf("ray%d endpoint/miss returned hit: %x %x", rayIndex, word(offset), word(offset+1))
		}

		return
	}

	if word(offset) != expected.triangle || word(offset+1) != expected.emitter {
		t.Fatalf("ray%d missing complete hit identity: triangle%x emitter%x", rayIndex, word(offset), word(offset+1))
	}

	value := func(slot int) float32 { return math.Float32frombits(word(slot)) }
	if normal := (Vec3{value(offset + 2), value(offset + 3), value(offset + 4)}); normal != expected.normal {
		t.Fatalf("ray%d normal%v want%v", rayIndex, normal, expected.normal)
	}

	if value(offset+5) != 2 {
		t.Fatalf("ray%d distance%g want2", rayIndex, value(offset+5))
	}

	if expected.mesh && (Vec3{value(offset + 6), value(offset + 7), value(offset + 8)}) != (Vec3{.25, .25, .5}) {
		t.Fatal("emissive triangle barycentrics changed", rayIndex)
	}
}

const payloadPredicateEntry = `
@compute @workgroup_size(1)
fn lm_reset(){lmIndirect[0]=(lp.count+63u)/64u;lmIndirect[1]=1u;lmIndirect[2]=1u;}
@compute @workgroup_size(64)
fn lm_intersect(@builtin(global_invocation_id) invocation:vec3u){
 let id=invocation.x;if id>=lp.count{return;}let ray=8u+id*8u;let o=tv(ray);let d=tv(ray+4u);let maximum=tf(ray+3u);
 let any=trace(o,d,maximum,true);let closest=trace(o,d,maximum,false);let at=id*56u;
 lpath.data[at]=select(0u,1u,occluded(o,d,maximum));
 lpath.data[at+1u]=any.triangle;lpath.data[at+2u]=any.emitter;
 for(var axis=0u;axis<3u;axis++){lpath.data[at+3u+axis]=bitcast<u32>(any.normal[axis]);lpath.data[at+7u+axis]=bitcast<u32>(any.bary[axis]);lpath.data[at+12u+axis]=bitcast<u32>(closest.normal[axis]);lpath.data[at+16u+axis]=bitcast<u32>(closest.bary[axis]);}
 lpath.data[at+6u]=bitcast<u32>(any.distance);lpath.data[at+10u]=closest.triangle;lpath.data[at+11u]=closest.emitter;lpath.data[at+15u]=bitcast<u32>(closest.distance);lpath.data[at+33u]=1u;
}
`
