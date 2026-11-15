package lightmapper

type referenceWelford struct{ mean, m2 float64 }

func (s *referenceWelford) add(value float32, count uint32) error {
	if !finite(value) || value < 0 || count == 0 {
		return ErrBounds
	}

	delta := float64(value) - s.mean
	s.mean += delta / float64(count)
	s.m2 += delta * (float64(value) - s.mean)

	return nil
}
func (s *referenceWelford) finish(count uint32) (mean, variance, second float32, secondValid bool, err error) {
	if count == 0 {
		err = ErrInvalid

		return
	}

	mean = float32(s.mean)
	if count > 1 {
		variance = float32(max(0, s.m2/float64(count-1)))
	}

	second = float32(s.mean*s.mean + s.m2/float64(count))
	if !finite(mean) || !finite(variance) {
		err = ErrBounds

		return
	}

	secondValid = finite(second)
	if !secondValid {
		second = 0
	}

	return
}
