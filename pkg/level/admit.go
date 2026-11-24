package level

import (
	"github.com/karty-game/ashura/internal/leveldata"
	"github.com/karty-game/ashura/internal/meshdata"
	"github.com/karty-game/ashura/internal/modeldata"
	"github.com/karty-game/ashura/pkg/model"
	"github.com/karty-game/ashura/pkg/texture"
	"github.com/karty-game/ashura/pkg/texture/ktx2"
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
	geometry, err := modeldata.DecodeVerified(prepared.payloads[page.Geometry.SHA256])
	if err != nil {
		return preparedPage{}, err
	}

	layout, err := leveldata.AdmitUVOwned(prepared.payloadBytes(page.UV.SHA256))
	if err != nil {
		return preparedPage{}, err
	}

	document := geometry.Document()
	if layout.ModelDigest() != geometry.Digest() || len(document.Textures) != 0 || len(document.Parts) != len(page.Materials) ||
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

	return preparedPage{description: page, layout: layout, asset: model.Admitted(prepared.chunk.ID, geometry)}, nil
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

		if prepared.imageInfo[source.SSBump.SHA256].format != ssbumpFormat(storage) {
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

	data, err := ktx2.Decode(prepared.payloadBytes(ref.SHA256))
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
