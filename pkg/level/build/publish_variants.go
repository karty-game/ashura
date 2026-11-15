//go:build !js

package levelbuild

import "github.com/karty-game/karty-webgpu/pkg/assetbuild"

// PublishVariants emits separately selected GPU profiles from the same baked
// inputs. Geometry, UV and matching texture payloads retain one publication owner.
// Every profile must supply a distinct nonempty Variant for its Go descriptor.
func PublishVariants(profiles ...PublishOptions) assetbuild.Plugin {
	return variantPublisher{profiles: append([]PublishOptions(nil), profiles...)}
}

type variantPublisher struct{ profiles []PublishOptions }

func (variantPublisher) Name() string { return "level-publish-variants" }
func (publisher variantPublisher) Plan(plan *assetbuild.Plan) error {
	const minimumVariants = 2
	if len(publisher.profiles) < minimumVariants {
		return ErrInvalid
	}

	seen := map[string]bool{}
	shared := map[string]fileRecord{}
	codecs := &codecActionCache{}
	atlases := &atlasActionCache{}
	pages := &packageActionCache{}
	profiles := make([]PublishOptions, 0, len(publisher.profiles))

	for _, profile := range publisher.profiles {
		if profile.Variant == "" || seen[profile.Variant] {
			return ErrInvalid
		}

		seen[profile.Variant] = true

		profile.sharedFiles = shared

		profile.sharedCodecs = codecs

		profile.sharedAtlases = atlases
		profile.sharedPages = pages

		profile, invalid := preparePublishOptions(profile)
		if invalid != nil {
			return invalid.Plan(plan)
		}

		profiles = append(profiles, profile)
	}

	if nativeVariantBatch(profiles) {
		return planNativeVariants(plan, profiles)
	}

	for _, profile := range profiles {
		if err := Publish(profile).Plan(plan); err != nil {
			return err
		}
	}

	return nil
}

func publishSharedFiles(plan *assetbuild.Plan, identity string, files map[string]fileRecord, shared map[string]fileRecord) error {
	for _, file := range sortedFiles(files) {
		if previous, exists := shared[file.Reference.Path]; exists {
			if previous != file {
				return ErrInvalid
			}

			continue
		}

		if err := addFile(plan, identity, file); err != nil {
			return err
		}

		if shared != nil {
			shared[file.Reference.Path] = file
		}
	}

	return nil
}
