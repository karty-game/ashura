//go:build js && wasm

package browser

import (
	"fmt"

	"github.com/karty-game/karty-webgpu/internal/platform"
	"github.com/karty-game/karty-webgpu/internal/render"
)

// RegisterStandard opts this driver into standard material pipelines and WGSL.
func RegisterStandard(host any) error {
	driver, err := materialFeatureDriver(host)
	if err != nil {
		return err
	}

	if !driver.standardPipeline.IsUndefined() || !driver.standardCulledPipeline.IsUndefined() {
		return fmt.Errorf("%s: %w", platform.StandardFeature, platform.ErrFeatureDuplicate)
	}

	return driver.createPipeline(driver.colorFormat, "standard.wgsl", render.StandardShader(), false)
}

// RegisterSource opts this driver into Source material pipelines and WGSL.
func RegisterSource(host any) error {
	driver, err := materialFeatureDriver(host)
	if err != nil {
		return err
	}

	if !driver.sourcePipeline.IsUndefined() || !driver.sourceCulledPipeline.IsUndefined() {
		return fmt.Errorf("%s: %w", platform.SourceFeature, platform.ErrFeatureDuplicate)
	}

	return driver.createPipeline(driver.colorFormat, "source.wgsl", render.SourceShader(), true)
}

// RegisterImpostors opts this driver into the prepared impostor pipeline and WGSL.
func RegisterImpostors(host any) error {
	driver, err := materialFeatureDriver(host)
	if err != nil {
		return err
	}

	if !driver.impostorPipeline.IsUndefined() {
		return fmt.Errorf("%s: %w", platform.ImpostorFeature, platform.ErrFeatureDuplicate)
	}

	return driver.createImpostorPipeline(driver.colorFormat)
}

func materialFeatureDriver(host any) (*Driver, error) {
	driver, ok := host.(*Driver)
	if !ok || driver == nil || driver.closed || driver.running || driver.device.IsUndefined() || driver.device.IsNull() {
		return nil, fmt.Errorf("browser renderer host: %w", platform.ErrFeatureInvalid)
	}

	return driver, nil
}

func (driver *Driver) requireMaterialRenderer(draw render.DrawCall) error {
	pipeline := driver.drawPipeline(draw)
	if !pipeline.IsUndefined() && !pipeline.IsNull() {
		return nil
	}

	return fmt.Errorf("%w: %s renderer", platform.ErrFeatureMissing, materialFeatureName(draw))
}

func materialFeatureName(draw render.DrawCall) string {
	if draw.Impostor != nil {
		return "impostor"
	}

	if draw.Source {
		return "Source material"
	}

	return "standard material"
}
