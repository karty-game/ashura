package main

type galleryPreset struct {
	key, label    string
	height, scale float32
}

type galleryCollection struct {
	key, label string
	presets    []galleryPreset
}

func galleryCollections() []galleryCollection {
	return []galleryCollection{
		{"featured", "Featured", []galleryPreset{
			{"solar-burst", "Solar Burst", 1.43, .012},
			{"hit-flash", "Hit Flash", 1.43, .012},
			{"pickup-sparkle", "Pickup Sparkle", 1.37, .015},
			{"fireworks", "Fireworks", .28, .012},
			{"water-splash", "Water Splash", .12, .013},
			{"ice-shatter", "Ice Shatter", 1.21, .013},
			{"arcane-portal", "Arcane Portal", 1.55, .016},
		}},
		{"impacts", "Bursts & Jets", []galleryPreset{
			{"dust-puff", "Dust Puff", .48, .022},
			{"hit-spark-star", "Hit Spark Star", 1.43, .011},
			{"laser-impact", "Laser Impact", 1.53, .014},
			{"rocket-exhaust", "Rocket Exhaust", 2.42, .018},
			{"shockwave-ring", "Shockwave Ring", .09, .014},
		}},
		{"magic", "Magic & Celebration", []galleryPreset{
			{"button-pop", "Button Pop", 1.43, .014},
			{"confetti", "Confetti", 2.15, .015},
			{"healing-aura", "Healing Aura", .42, .019},
			{"level-up-burst", "Level Up Burst", .45, .015},
			{"magic-burst", "Magic Burst", 1.55, .015},
		}},
		{"weather", "Weather & Water", []galleryPreset{
			{"blizzard", "Blizzard", 3.32, .009},
			{"bubbles", "Bubbles", .45, .026},
			{"fog-bank", "Fog Bank", .52, .012},
			{"geyser", "Geyser", .12, .012},
			{"snow", "Snow", 3.15, .010},
			{"waterfall-mist", "Waterfall Mist", 3.55, .010},
		}},
		{"fire", "Fire & Haze", []galleryPreset{
			{"cinder-fall", "Cinder Fall", .29, .010},
			{"fire", "Fire", .18, .017},
			{"poison-cloud", "Poison Cloud", .60, .015},
			{"torch-flame", "Torch Flame", .28, .016},
			{"varied-embers", "Varied Embers", .35, .014},
		}},
	}
}
