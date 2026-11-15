package engine

import "testing"

func TestMSAADefaultAndOptOut(t *testing.T) {
	t.Parallel()

	const unsupported uint32 = 2
	for _, samples := range []uint32{0, 1, defaultMSAASamples, unsupported} {
		app := NewApp(AppConfig{MSAASamples: samples})

		wanted := defaultMSAASamples
		if samples == 1 {
			wanted = 1
		}

		if app.config.MSAASamples != wanted {
			t.Fatalf("sample count %d -> %d, want %d", samples, app.config.MSAASamples, wanted)
		}
	}
}
