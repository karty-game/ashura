package level

import (
	"encoding/hex"

	"github.com/karty-game/karty-webgpu/internal/leveldata"
	"github.com/karty-game/karty-webgpu/internal/meshdata"
	"github.com/karty-game/karty-webgpu/internal/modeldata"
	"github.com/karty-game/karty-webgpu/pkg/model"
	"github.com/karty-game/karty-webgpu/pkg/texture"
	"github.com/karty-game/karty-webgpu/pkg/texture/ktx2"
)

func (prepared *PreparedChunk) admit() error {
	for _, page := range prepared.chunk.Pages {
		admitted, err := prepared.admitPage(page)
		if err != nil {
			return err
		}

		prepared.pages = append(prepared.pages, admitted)
	}

	return nil
}

func (prepared *PreparedChunk) admitPage(page Page) (preparedPage, error) {
	geometry := prepared.files[page.Geometry.SHA256]

	document, err := modeldata.Decode(geometry)
	if err != nil {
		return preparedPage{}, err
	}

	layout, err := leveldata.AdmitUVOwned(prepared.files[page.UV.SHA256])
	if err != nil {
		return preparedPage{}, err
	}

	digest := layout.ModelDigest()
	if hex.EncodeToString(digest[:]) != page.Geometry.SHA256 || len(document.Textures) != 0 || len(document.Parts) != len(page.Materials) ||
		layout.PartCount() != len(page.Materials) {
		return preparedPage{}, leveldata.ErrFormat
	}

	for index, part := range document.Parts {
		if part.Name != page.Materials[index].Name || len(layout.PartUV(index)) != len(part.Vertices)/meshdata.VertexStride*8 ||
			part.Material.AlphaMask ||
			part.Material.Albedo != -1 ||
			part.Material.Normal != -1 ||
			part.Material.AO != -1 {
			return preparedPage{}, leveldata.ErrFormat
		}
	}

	lightmapFormat, lightmapSpace := lightmapImageProfile(page)
	for _, ref := range page.Maps[:page.MapCount()] {
		if err = prepared.admitImage(ref, lightmapSpace, page.Width, page.Height); err != nil {
			return preparedPage{}, err
		}

		if prepared.imageInfo[ref.SHA256].format != lightmapFormat {
			return preparedPage{}, leveldata.ErrFormat
		}
	}

	for _, source := range page.Materials {
		if err = prepared.admitMaterialImages(source, page.Storage); err != nil {
			return preparedPage{}, err
		}
	}

	return preparedPage{description: page, layout: layout, geometry: geometry,
		asset: model.Verified(prepared.chunk.ID, prepared.verified[page.Geometry.SHA256])}, nil
}

func (prepared *PreparedChunk) admitMaterialImages(source Material, storage string) error {
	if source.Albedo != nil {
		if err := prepared.admitImage(*source.Albedo, texture.SRGB, 0, 0); err != nil {
			return err
		}

		if !matchesAlbedoEncoding(source.AlbedoEncoding, prepared.imageInfo[source.Albedo.SHA256].format) {
			return leveldata.ErrFormat
		}
	}

	if source.SSBump != nil {
		if err := prepared.admitImage(*source.SSBump, texture.Linear, 0, 0); err != nil {
			return err
		}

		expected := texture.RGBA8

		switch storage {
		case nativeHDRStorage, leveldata.StorageBC1LDR, leveldata.StorageBC7LDR:
			expected = texture.BC7
		case mobileStorage:
			expected = texture.ASTC4x4
		}

		if prepared.imageInfo[source.SSBump.SHA256].format != expected {
			return leveldata.ErrFormat
		}
	}

	return nil
}

func (prepared *PreparedChunk) admitImage(ref Reference, space texture.ColorSpace, width, height uint32) error {
	info, found := prepared.imageInfo[ref.SHA256]
	if !found {
		var err error

		info, err = prepared.readImageMetadata(ref)
		if err != nil {
			return err
		}

		prepared.imageInfo[ref.SHA256] = info
		prepared.imageRefs = append(prepared.imageRefs, ref)
	}

	maximum := 1024
	if width > 0 {
		maximum = 2048
	}

	if info.size != ref.Size || info.space != space || info.width > maximum || info.height > maximum ||
		width > 0 && (info.width != int(width) || info.height != int(height) || info.levels != 1) {
		return leveldata.ErrFormat
	}

	return nil
}

func (prepared *PreparedChunk) readImageMetadata(ref Reference) (imageMetadata, error) {
	if info, resident := prepared.resident[ref.SHA256]; resident {
		return info, nil
	}

	if data, transcoded := prepared.images[ref.SHA256]; transcoded {
		return metadata(data, ref.Size), nil
	}

	data, err := ktx2.Decode(prepared.files[ref.SHA256])
	if err != nil {
		return imageMetadata{}, err
	}

	prepared.images[ref.SHA256] = data

	return metadata(data, ref.Size), nil
}

func matchesAlbedoEncoding(encoding string, format texture.Format) bool {
	switch encoding {
	case "":
		return format == texture.RGBA8
	case etc1sEncoding:
		return format == texture.BC1 || format == texture.ETC2RGB
	case bc1Encoding:
		return format == texture.BC1
	case etc2Encoding:
		return format == texture.ETC2RGB
	default:
		return false
	}
}
