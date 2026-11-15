package lightmapper

import "math"

//nolint:gochecknoglobals // Prime bases are the immutable versioned CPU/WGSL sampling table.
var samplePrimes = [64]uint32{
	2,
	3,
	5,
	7,
	11,
	13,
	17,
	19,
	23,
	29,
	31,
	37,
	41,
	43,
	47,
	53,
	59,
	61,
	67,
	71,
	73,
	79,
	83,
	89,
	97,
	101,
	103,
	107,
	109,
	113,
	127,
	131,
	137,
	139,
	149,
	151,
	157,
	163,
	167,
	173,
	179,
	181,
	191,
	193,
	197,
	199,
	211,
	223,
	227,
	229,
	233,
	239,
	241,
	251,
	257,
	263,
	269,
	271,
	277,
	281,
	283,
	293,
	307,
	311,
}

//nolint:mnd // These fixed integer-mixing constants match the compute sampling contract.
func sampleHash(x uint32) uint32 {
	x = (x ^ (x >> 16)) * 0x7feb352d
	x = (x ^ (x >> 15)) * 0x846ca68b

	return x ^ (x >> 16)
}

// SampleDimension is a deterministically rotated Halton dimension. Its identity
// excludes dispatch size/order and output atlas packing. An independent seed
// must be used for pilot, estimation and representation verification passes.
//
//nolint:gosec,mnd //  Receiver identities are deliberately split into low/high32-bit hash words. Eight per-bounce dimensions and a 24-bit Cranley-Patterson rotation match the WGSL sampling contract.
func SampleDimension(receiverID uint64, seed, sample, bounce, dimension uint32) float32 {
	sequenceDimension := bounce*8 + dimension
	base := samplePrimes[sequenceDimension%64]
	index := sample + 1
	factor := float32(1) / float32(base)

	value := float32(0)
	for index > 0 {
		value += float32(index%base) * factor
		index /= base
		factor /= float32(base)
	}

	rotation := float32(
		sampleHash(uint32(receiverID)^sampleHash(uint32(receiverID>>32))^seed^(sequenceDimension*0x9e3779b9))&0xffffff,
	) / 16777216
	result := value + rotation

	return min(float32(math.Nextafter32(1, 0)), result-float32(math.Floor(float64(result))))
}
