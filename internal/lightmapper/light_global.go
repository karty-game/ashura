package lightmapper

const minimumLightProposalPower float32 = 1e-12

func globalEmitter(kind EmitterKind) bool {
	return kind == Directional || kind == Sun || kind == EnvironmentSource
}

// Global terms are incoming-irradiance estimates, not powers divided by an
// artificial million-unit box. Their exact selection mass is still recovered
// from the same hierarchy used for finite sources.
//
//nolint:exhaustive // Only globally supported emitter kinds call this proposal helper.
func (p *PreparedScene) globalEmitterPower(emitter Emitter, estimate float32) float32 {
	power := estimate

	switch emitter.Kind {
	case Directional:
		power = luminance(emitter.Intensity)
	case Sun:
		power = luminance(emitter.Intensity) * (2 * lightPi * sunCap(emitter.Radius))
	case EnvironmentSource:
		if p.source.Environment.LegacyIrradiance {
			power /= lightPi
		}
	}

	return max(power, minimumLightProposalPower)
}

func (p *PreparedScene) lightPowerScale() float32 {
	root := p.lightNodes[0]

	return max(root.Power, root.GlobalPower)
}

func (p *PreparedScene) lightNodeSupport(emitter Emitter) (Vec3, Vec3, float32, float32) {
	low, high, power := p.emitterBounds(emitter)
	if globalEmitter(emitter.Kind) {
		return Vec3{}, Vec3{}, 0, p.globalEmitterPower(emitter, power)
	}

	return low, high, power, 0
}

func lightNodeHasFinite(node LightNode) bool { return node.Power > 0 }

func globalPowerFinite(node LightNode) bool {
	return finite(node.GlobalPower) && node.GlobalPower >= 0 && (node.Power > 0 || node.GlobalPower > 0)
}

func maxFiniteBounds(low, high Vec3, count int) (Vec3, Vec3) {
	if count == 0 {
		return Vec3{}, Vec3{}
	}

	return low, high
}
