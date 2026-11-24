package lightmapper

import (
	"fmt"
	"math"
	"slices"
)

// PreparedScene owns validated immutable source arrays and deterministic spatial
// hierarchies. Borrowed accessor results must never be mutated by a consumer.
type PreparedScene struct {
	source             Scene
	hash               string
	geometryHash       string
	nodes              []BVHNode
	order              []uint32
	lightNodes         []LightNode
	lightOrder         []uint32
	lightLeaves        []uint32
	lightParents       []uint32
	triangleEmitters   []int32
	environmentEmitter int32
	environmentRows    []float32
	environmentColumns []float32
}

func (prepared *PreparedScene) Source() Scene           { return prepared.source }
func (prepared *PreparedScene) Hash() string            { return prepared.hash }
func (prepared *PreparedScene) Nodes() []BVHNode        { return prepared.nodes }
func (prepared *PreparedScene) Order() []uint32         { return prepared.order }
func (prepared *PreparedScene) LightNodes() []LightNode { return prepared.lightNodes }
func (prepared *PreparedScene) TriangleEmitter(index uint32) int32 {
	return prepared.triangleEmitters[index]
}

func (prepared *PreparedScene) EnvironmentRows() []float32    { return prepared.environmentRows }
func (prepared *PreparedScene) EnvironmentColumns() []float32 { return prepared.environmentColumns }

func PrepareScene(source Scene) (*PreparedScene, error) {
	prepared, err := admitSceneSource(source, false)
	if err != nil {
		return nil, err
	}

	prepared.buildGeometry()

	if err = prepared.rebuildLighting(); err != nil {
		return nil, err
	}

	if err = prepared.validateSurfaceOwnership(); err != nil {
		return nil, err
	}

	if err = prepared.finishSceneIdentity(); err != nil {
		return nil, err
	}

	return prepared, nil
}

func (prepared *PreparedScene) GeometryHash() string { return prepared.geometryHash }

func admitSceneSource(source Scene, owned bool) (*PreparedScene, error) {
	if source.ID == "" || source.Space == "" || !finite(source.MetresPerUnit) || source.MetresPerUnit <= 0 ||
		len(source.Triangles) > 1<<20 ||
		len(source.Emitters) > 1<<20 || len(source.Materials) > 1<<20 || len(source.Textures) > 1<<20 {
		return nil, ErrInvalid
	}

	if !owned {
		source.Triangles = slices.Clone(source.Triangles)
		source.Materials = slices.Clone(source.Materials)
		source.Emitters = slices.Clone(source.Emitters)
		source.Textures = slices.Clone(source.Textures)
	}

	if err := admitSceneTextures(&source, owned); err != nil {
		return nil, err
	}

	if err := admitSceneMaterials(source); err != nil {
		return nil, err
	}

	if err := admitSceneTriangles(&source); err != nil {
		return nil, err
	}

	if !positiveVec(source.Environment.Sky) || !positiveVec(source.Environment.Ground) ||
		uint64(source.Environment.Texture) > uint64(len(source.Textures)) {
		return nil, ErrInvalid
	}

	prepared := &PreparedScene{source: source, triangleEmitters: make([]int32, len(source.Triangles)), environmentEmitter: -1}
	for i := range prepared.triangleEmitters {
		prepared.triangleEmitters[i] = -1
	}

	if err := prepared.prepareEmitters(); err != nil {
		return nil, err
	}

	return prepared, nil
}

func (prepared *PreparedScene) Reflectance(material uint32, coordinates Vec2) Vec3 {
	m := prepared.source.Materials[material]
	if m.Texture == 0 {
		return m.Reflectance
	}

	return Mul(m.Reflectance, sampleTexture(prepared.source.Textures[m.Texture-1], coordinates))
}

func sampleTexture(t Texture, coordinates Vec2) Vec3 {
	return sampleTextureMode(t, coordinates, false)
}

//nolint:mnd // Bilinear interpolation addresses texture samples at their half-texel centers.
func sampleTextureMode(texture Texture, coordinates Vec2, clampV bool) Vec3 {
	textureU, textureV := float64(coordinates[0]), float64(coordinates[1])
	textureU -= math.Floor(textureU)

	if clampV {
		textureV = max(0, min(1, textureV))
	} else {
		textureV -= math.Floor(textureV)
	}

	column := textureU*float64(texture.Width) - .5
	row := textureV*float64(texture.Height) - .5
	firstX, firstY := int64(math.Floor(column)), int64(math.Floor(row))
	fractionX, fractionY := float32(column-float64(firstX)), float32(row-float64(firstY))
	get := func(column, row int64) Vec3 {
		column = (column%int64(texture.Width) + int64(texture.Width)) % int64(texture.Width)
		if clampV {
			row = max(0, min(int64(texture.Height)-1, row))
		} else {
			row = (row%int64(texture.Height) + int64(texture.Height)) % int64(texture.Height)
		}

		return texture.Pixels[row*int64(texture.Width)+column]
	}

	return mixVec(
		mixVec(get(firstX, firstY), get(firstX+1, firstY), fractionX),
		mixVec(get(firstX, firstY+1), get(firstX+1, firstY+1), fractionX),
		fractionY,
	)
}

func admitSceneTextures(source *Scene, owned bool) error {
	const linearPixelBytes = 3 * 4

	var bytes uint64

	for index, texture := range source.Textures {
		count := uint64(texture.Width) * uint64(texture.Height)
		if texture.Width == 0 || texture.Height == 0 || texture.Width > 8192 || texture.Height > 8192 || count > 1<<24 ||
			count != uint64(len(texture.Pixels)) {
			return ErrBounds
		}

		bytes += count * linearPixelBytes
		if bytes > 256<<20 {
			return ErrBounds
		}

		for _, v := range texture.Pixels {
			if !positiveVec(v) {
				return ErrInvalid
			}
		}

		if !owned {
			source.Textures[index].Pixels = slices.Clone(texture.Pixels)
		}
	}

	return nil
}
func admitSceneMaterials(source Scene) error {
	materialIDs := make(map[string]bool, len(source.Materials))
	for _, material := range source.Materials {
		if len(material.ID) > 4096 || material.ID != "" && materialIDs[material.ID] {
			return ErrInvalid
		}

		if material.ID != "" {
			materialIDs[material.ID] = true
		}

		if !positiveVec(material.Reflectance) || maxComponent(material.Reflectance) > 1 || !positiveVec(material.Emission) ||
			uint64(material.Texture) > uint64(len(source.Textures)) {
			return ErrInvalid
		}

		if material.Texture > 0 {
			for _, v := range source.Textures[material.Texture-1].Pixels {
				if maxComponent(v) > 1 {
					return ErrInvalid
				}
			}
		}
	}

	return nil
}
func admitSceneTriangles(source *Scene) error {
	ids := make(map[uint64]struct{}, len(source.Triangles))
	for triangleIndex, triangle := range source.Triangles {
		if _, exists := ids[triangle.ID]; exists {
			return fmt.Errorf("duplicate triangle identity: %w", ErrInvalid)
		}

		ids[triangle.ID] = struct{}{}
		if uint64(triangle.Material) >= uint64(len(source.Materials)) {
			return ErrInvalid
		}

		for corner := range 3 {
			if !validVec(triangle.Positions[corner]) || !validVec(triangle.Normals[corner]) || !finite(triangle.UV[corner][0]) ||
				!finite(triangle.UV[corner][1]) {
				return ErrInvalid
			}
		}

		geometric := Normalize(Cross(Sub(triangle.Positions[1], triangle.Positions[0]), Sub(triangle.Positions[2], triangle.Positions[0])))
		if geometric == (Vec3{}) || !validVec(geometric) {
			return fmt.Errorf("degenerate triangle%d: %w", triangle.ID, ErrInvalid)
		}

		for corner := range 3 {
			normal := triangle.Normals[corner]

			length := Length(normal)
			if length == 0 {
				normal = geometric
			} else if math.Abs(float64(length-1)) > float64(canonicalUnitTolerance) {
				normal = Normalize(normal)
			}

			source.Triangles[triangleIndex].Normals[corner] = normal
		}
	}

	return nil
}
