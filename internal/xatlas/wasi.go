//go:build !js

//nolint:mnd,gosec // Pinned wasm32 xatlas C-API layout and bounded protocol words.
package xatlas

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/binary"
	"fmt"
	"math"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

//go:embed third_party/xatlas.wasm
var xatlasWASM []byte

type wasiSession struct {
	// Only initialization mutates runtime/compiled; requests own separate instances.
	prepareMu sync.Mutex
	runtime   wazero.Runtime
	compiled  wazero.CompiledModule
	//nolint:containedctx // Build-owned teardown is independent of a canceled unwrap.
	cleanup context.Context
}
type atlasGuest struct {
	//nolint:containedctx // This guest view exists only for the current request.
	ctx    context.Context
	module api.Module
}
type rawVertex struct {
	original    uint32
	chart, page int32
	uv          [2]float32
}
type rawMesh struct {
	vertices []rawVertex
	indices  []uint32
	charts   [][]uint32
}
type rawAtlas struct {
	width, height, pages, charts uint32
	meshes                       []rawMesh
	image                        []byte
}

func (session *wasiSession) close() error {
	if session.runtime != nil {
		return session.runtime.Close(session.cleanup)
	}

	return nil
}

func (session *wasiSession) execute(ctx context.Context, input []byte) (output []byte, err error) {
	if len(input) > 128<<20 {
		return nil, errFormat
	}

	defer func() {
		if failure := recover(); failure != nil {
			if cause, ok := failure.(error); ok {
				err = cause
			} else {
				err = fmt.Errorf("xatlas WASI: %v: %w", failure, errFormat)
			}

			output = nil
		}

		if ctx.Err() != nil {
			err = ctx.Err()
			output = nil
		}
	}()

	if err = session.prepare(ctx); err != nil {
		return nil, err
	}

	module, err := session.runtime.InstantiateModule(ctx, session.compiled, wazero.NewModuleConfig().WithName("").WithStartFunctions())
	if err != nil {
		return nil, err
	}
	defer func() { _ = module.Close(context.WithoutCancel(ctx)) }()

	guest := atlasGuest{ctx, module}
	reader := reader{data: input}
	magic, parts, resolution, padding := reader.word(), reader.word(), reader.word(), reader.word()
	physical := magic == 0x3258414b

	density, maxPages := float32(0), uint32(1)
	if physical {
		density = math.Float32frombits(reader.word())
		maxPages = reader.word()
	}

	atlas := guest.call("xatlasCreate")
	sources := guest.addMeshes(atlas, &reader, parts, physical)
	pack := guest.generate(atlas, resolution, padding, density, physical)

	result := guest.atlas(atlas, parts, maxPages, resolution)
	if physical {
		result = guest.repair(atlas, pack, result, sources, density/1024, parts, maxPages, resolution)
	}

	output = encodeRawAtlas(result, magic, physical, density)

	return output, nil
}

func (session *wasiSession) prepare(ctx context.Context) (err error) {
	session.prepareMu.Lock()
	defer session.prepareMu.Unlock()

	if session.runtime == nil {
		// Do not retain an action context and its input snapshot across requests.
		session.cleanup = context.Background()

		if fmt.Sprintf("%x", sha256.Sum256(xatlasWASM)) != "0036793f9233f4e2ab190cf300c644836147d79fb972d1a13c0214c0e1af5761" {
			return errFormat
		}

		session.runtime = wazero.NewRuntimeWithConfig(
			ctx,
			wazero.NewRuntimeConfigCompiler().WithMemoryLimitPages(10240).WithCloseOnContextDone(true),
		)

		_, err = session.runtime.NewHostModuleBuilder("env").
			NewFunctionBuilder().
			WithFunc(func(ctx context.Context, _, _, _ uint32) uint32 {
				if ctx.Err() != nil {
					return 0
				}

				return 1
			}).
			Export("js_progress").
			Instantiate(ctx)
		if err != nil {
			_ = session.runtime.Close(context.WithoutCancel(ctx))
			session.runtime = nil

			return err
		}
	}

	if session.compiled == nil {
		session.compiled, err = session.runtime.CompileModule(ctx, xatlasWASM)
		if err != nil {
			return err
		}
	}

	return nil
}

func scaleAtlasWorkspace(data []byte) {
	var origin [3]float64
	for axis := range 3 {
		origin[axis] = float64(math.Float32frombits(binary.LittleEndian.Uint32(data[axis*4:])))
	}

	for vertex := 0; vertex < len(data); vertex += 32 {
		for axis := range 3 {
			offset := vertex + axis*4

			value := float32((float64(math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))) - origin[axis]) * 1024)
			if !finite(value) {
				panic(errFormat)
			}

			binary.LittleEndian.PutUint32(data[offset:], math.Float32bits(value))
		}
	}
}

func (guest atlasGuest) call(name string, args ...uint64) uint32 {
	function := guest.module.ExportedFunction(name)
	if function == nil {
		panic(errFormat)
	}

	values, err := function.Call(guest.ctx, args...)
	if err != nil {
		panic(err)
	}

	if len(values) == 0 {
		return 0
	}

	return uint32(values[0])
}
func (guest atlasGuest) alloc(size uint32) uint32 {
	if size == 0 || size > 128<<20 {
		panic(errFormat)
	}

	address := guest.call("malloc", uint64(size))
	if address == 0 {
		panic(errFormat)
	}

	return address
}
func (guest atlasGuest) copy(data []byte) uint32 {
	address := guest.alloc(uint32(len(data)))
	guest.write(address, data)

	return address
}
func (guest atlasGuest) write(address uint32, data []byte) {
	if !guest.module.Memory().Write(address, data) {
		panic(errFormat)
	}
}
func (guest atlasGuest) put(address, value uint32) {
	if !guest.module.Memory().WriteUint32Le(address, value) {
		panic(errFormat)
	}
}
func (guest atlasGuest) putf(address uint32, value float32) {
	guest.put(address, math.Float32bits(value))
}
func (guest atlasGuest) word(address uint32) uint32 {
	value, ok := guest.module.Memory().ReadUint32Le(address)
	if !ok {
		panic(errFormat)
	}

	return value
}
func (guest atlasGuest) float(address uint32) float32 {
	return math.Float32frombits(guest.word(address))
}
func (guest atlasGuest) read(address, size uint32) []byte {
	if size > 128<<20 {
		panic(errFormat)
	}

	data, ok := guest.module.Memory().Read(address, size)
	if !ok {
		panic(errFormat)
	}

	return bytes.Clone(data)
}

func (guest atlasGuest) atlas(address, parts, maxPages, resolution uint32) rawAtlas {
	result := rawAtlas{
		width:  guest.word(address + 12),
		height: guest.word(address + 16),
		pages:  guest.word(address + 20),
		charts: guest.word(address + 24),
	}
	if result.width != resolution || result.height != resolution || result.pages == 0 || result.pages > maxPages || result.charts == 0 ||
		result.charts > 1<<20 ||
		guest.word(address+28) != parts ||
		uint64(result.width)*uint64(result.height)*uint64(result.pages)*4 > 128<<20 {
		panic(errFormat)
	}

	meshes := guest.word(address + 4)

	var totalVertices, totalIndices, totalCharts uint64

	for part := range parts {
		base := meshes + part*24
		chartCount, indexCount, vertexCount := guest.word(base+12), guest.word(base+16), guest.word(base+20)

		totalVertices += uint64(vertexCount)
		totalIndices += uint64(indexCount)

		totalCharts += uint64(chartCount)
		if totalVertices > 1<<20 || totalIndices > 3<<20 || totalCharts > uint64(result.charts) {
			panic(errFormat)
		}

		mesh := rawMesh{vertices: make([]rawVertex, vertexCount), indices: make([]uint32, indexCount), charts: make([][]uint32, chartCount)}

		vertexPointer, indexPointer, chartPointer := guest.word(base+8), guest.word(base+4), guest.word(base)
		for i := range mesh.vertices {
			p := vertexPointer + uint32(i)*20
			mesh.vertices[i] = rawVertex{
				original: guest.word(p + 16),
				chart:    int32(guest.word(p + 4)),
				page:     int32(guest.word(p)),
				uv:       [2]float32{guest.float(p + 8), guest.float(p + 12)},
			}
		}

		for i := range mesh.indices {
			mesh.indices[i] = guest.word(indexPointer + uint32(i)*4)
			if mesh.indices[i] >= vertexCount {
				panic(errFormat)
			}
		}

		guest.readCharts(&mesh, chartPointer, indexCount)

		result.meshes = append(result.meshes, mesh)
	}

	result.image = guest.read(guest.word(address), result.width*result.height*result.pages*4)

	return result
}

func encodeRawAtlas(atlas rawAtlas, magic uint32, physical bool, density float32) []byte {
	var output []byte

	word := func(value uint32) {
		output = binary.LittleEndian.AppendUint32(output, value)
		if len(output) > 128<<20 {
			panic(errFormat)
		}
	}
	for _, value := range []uint32{magic + 0xe00, lenToWord(len(atlas.meshes)), atlas.width, atlas.height, atlas.charts} {
		word(value)
	}

	if physical {
		word(atlas.pages)
		word(math.Float32bits(density))
	}

	for _, mesh := range atlas.meshes {
		word(lenToWord(len(mesh.vertices)))
		word(lenToWord(len(mesh.indices)))

		for _, vertex := range mesh.vertices {
			word(vertex.original)
			word(uint32(vertex.chart))

			if physical {
				word(uint32(vertex.page))
			}

			word(math.Float32bits(vertex.uv[0]))
			word(math.Float32bits(vertex.uv[1]))
		}

		for _, index := range mesh.indices {
			word(index)
		}
	}

	if len(output)+len(atlas.image) > 128<<20 {
		panic(errFormat)
	}

	return append(output, atlas.image...)
}
func lenToWord(value int) uint32 { return uint32(value) }

func (guest atlasGuest) addMeshes(atlas uint32, reader *reader, parts uint32, physical bool) [][]byte {
	sources := make([][]byte, parts)

	var totalVertices, totalIndices uint64

	for part := range parts {
		vertexCount, indexCount := reader.word(), reader.word()
		size := uint64(vertexCount)*32 + uint64(indexCount)*4
		totalVertices += uint64(vertexCount)

		totalIndices += uint64(indexCount)
		if totalVertices > 1<<20 || totalIndices > 3<<20 {
			panic(errFormat)
		}

		if vertexCount == 0 || indexCount == 0 || size > uint64(len(reader.data)-reader.offset) {
			panic(errFormat)
		}

		vertices := bytes.Clone(reader.data[reader.offset : reader.offset+int(vertexCount)*32])
		reader.offset += int(vertexCount) * 32
		indices := reader.data[reader.offset : reader.offset+int(indexCount)*4]
		reader.offset += int(indexCount) * 4

		if physical {
			scaleAtlasWorkspace(vertices)
		}

		sources[part] = vertices
		vptr, iptr, decl := guest.copy(vertices), guest.copy(indices), guest.alloc(64)
		guest.call("xatlasMeshDeclInit", uint64(decl))

		for _, field := range [][2]uint32{{0, vptr}, {4, vptr + 12}, {8, vptr + 24}, {12, iptr}, {28, vertexCount}, {32, 32}, {36, 32}, {40, 32}, {44, indexCount}, {56, 1}} {
			guest.put(decl+field[0], field[1])
		}

		if guest.call("xatlasAddMesh", uint64(atlas), uint64(decl), uint64(parts)) != 0 {
			panic(errFormat)
		}

		guest.call("free", uint64(vptr))
		guest.call("free", uint64(iptr))
		guest.call("free", uint64(decl))
	}

	return sources
}

func (guest atlasGuest) generate(atlas, resolution, padding uint32, density float32, physical bool) uint32 {
	charts, pack := guest.alloc(44), guest.alloc(24)
	guest.call("xatlasChartOptionsInit", uint64(charts))
	guest.call("xatlasPackOptionsInit", uint64(pack))
	guest.put(pack+4, padding)
	guest.put(pack+12, resolution)
	guest.write(pack+19, []byte{1})

	if physical {
		d := density / 1024
		usable := float32(resolution-padding*2-2) / d
		guest.putf(charts+4, usable*usable*.75)
		guest.putf(charts+8, usable*1.5)
		guest.putf(pack+8, d)
	}

	guest.call("xatlasComputeCharts", uint64(atlas), uint64(charts))
	guest.call("xatlasPackCharts", uint64(atlas), uint64(pack))

	if !physical {
		extent := max(guest.word(atlas+12), guest.word(atlas+16))

		densityEstimate := guest.float(atlas+32) * float32(resolution) / float32(extent) * .9
		if extent == 0 || !finite(densityEstimate) || densityEstimate <= 0 {
			panic(errFormat)
		}

		for range 24 {
			guest.putf(pack+8, densityEstimate)
			guest.call("xatlasPackCharts", uint64(atlas), uint64(pack))

			if guest.word(atlas+20) == 1 {
				break
			}

			densityEstimate *= .8
		}
	}

	return pack
}

func (guest atlasGuest) readCharts(mesh *rawMesh, chartPointer, indexCount uint32) {
	var totalFaces uint64

	seen := make([]bool, indexCount/3)

	for chartIndex := range mesh.charts {
		p := chartPointer + uint32(chartIndex)*20

		count, faces := guest.word(p+8), guest.word(p)

		totalFaces += uint64(count)
		if totalFaces > uint64(indexCount/3) {
			panic(errFormat)
		}

		mesh.charts[chartIndex] = make([]uint32, count)
		for faceIndex := range mesh.charts[chartIndex] {
			face := guest.word(faces + uint32(faceIndex)*4)
			if face >= indexCount/3 || seen[face] {
				panic(errFormat)
			}

			seen[face] = true
			mesh.charts[chartIndex][faceIndex] = face
		}
	}
}
