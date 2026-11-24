//go:build !js

package build

import "encoding/json"

type document struct {
	SchemaVersion int               `json:"schemaVersion"`
	Meta          json.RawMessage   `json:"meta"`
	Duration      float64           `json:"duration"`
	Looping       bool              `json:"looping"`
	Seed          uint32            `json:"seed"`
	Params        []json.RawMessage `json:"params"`
	Textures      map[string]string `json:"textures"`
	Layers        []layer           `json:"layers"`
}

type layer struct {
	ID                 string  `json:"id"`
	Name               string  `json:"name"`
	Enabled            bool    `json:"enabled"`
	Blend              string  `json:"blend"`
	Space              string  `json:"space"`
	InheritVelocity    float64 `json:"inheritVelocity"`
	AttractorInfluence float64 `json:"attractorInfluence"`
	Texture            struct {
		Ref    string          `json:"ref"`
		Frames json.RawMessage `json:"frames"`
	} `json:"texture"`
	Emission struct {
		RateOverTime          track           `json:"rateOverTime"`
		RateOverTimeParam     *string         `json:"rateOverTimeParam"`
		RateOverDistanceParam *string         `json:"rateOverDistanceParam"`
		RateOverDistance      json.RawMessage `json:"rateOverDistance"`
		Bursts                []burst         `json:"bursts"`
		Delay                 float64         `json:"delay"`
		Prewarm               bool            `json:"prewarm"`
		MaxParticles          uint32          `json:"maxParticles"`
	} `json:"emission"`
	Shape   shape `json:"shape"`
	Initial struct {
		Life            track   `json:"life"`
		Speed           track   `json:"speed"`
		Size            track   `json:"size"`
		Rotation        track   `json:"rotation"`
		AngularVelocity track   `json:"angularVelocity"`
		LifeParam       *string `json:"lifeParam"`
		SpeedParam      *string `json:"speedParam"`
		SizeParam       *string `json:"sizeParam"`
	} `json:"initial"`
	OverLifetime struct {
		Size     *track `json:"size"`
		Rotation *track `json:"rotation"`
		Color    struct {
			Keys []gradientKey `json:"keys"`
		} `json:"color"`
		Velocity struct {
			Gravity struct {
				X float64 `json:"x"`
				Y float64 `json:"y"`
			} `json:"gravity"`
			GravityParam    *string         `json:"gravityParam"`
			Drag            *track          `json:"drag"`
			SpeedMultiplier json.RawMessage `json:"speedMultiplier"`
			X               json.RawMessage `json:"x"`
			Y               json.RawMessage `json:"y"`
			Orbital         json.RawMessage `json:"orbital"`
			Radial          json.RawMessage `json:"radial"`
		} `json:"velocity"`
	} `json:"overLifetime"`
	Noise *struct {
		Strength    track   `json:"strength"`
		Frequency   float64 `json:"frequency"`
		ScrollSpeed float64 `json:"scrollSpeed"`
		Octaves     int     `json:"octaves"`
	} `json:"noise"`
	TintParam      *string         `json:"tintParam"`
	OpacityParam   *string         `json:"opacityParam"`
	LimitVelocity  json.RawMessage `json:"limitVelocity"`
	Wind           json.RawMessage `json:"wind"`
	BySpeed        json.RawMessage `json:"bySpeed"`
	ByEmitterSpeed json.RawMessage `json:"byEmitterSpeed"`
	StartColor     json.RawMessage `json:"startColor"`
	RandomFlip     json.RawMessage `json:"randomFlip"`
	Render         *renderSettings `json:"render"`
	Dissolve       json.RawMessage `json:"dissolve"`
	Collision      json.RawMessage `json:"collision"`
	KillZones      json.RawMessage `json:"killZones"`
	Attractor      json.RawMessage `json:"attractor"`
	SubEmitters    json.RawMessage `json:"subEmitters"`
	Trail          json.RawMessage `json:"trail"`
}

type renderSettings struct {
	Align      string  `json:"align"`
	SpeedScale float64 `json:"speedScale"`
	MinStretch float64 `json:"minStretch"`
	MaxStretch float64 `json:"maxStretch"`
}

type burst struct {
	Time        float64 `json:"time"`
	Spread      float64 `json:"spread"`
	Interval    float64 `json:"interval"`
	Count       uint32  `json:"count"`
	Cycles      uint32  `json:"cycles"`
	Probability float64 `json:"probability"`
}

type shape struct {
	Kind        string          `json:"kind"`
	EmitFrom    string          `json:"emitFrom"`
	ArcMode     string          `json:"arcMode"`
	Radius      float64         `json:"radius"`
	InnerRadius float64         `json:"innerRadius"`
	Arc         float64         `json:"arc"`
	ArcSpeed    float64         `json:"arcSpeed"`
	Direction   float64         `json:"direction"`
	Spread      float64         `json:"spread"`
	Width       float64         `json:"width"`
	Height      float64         `json:"height"`
	Length      float64         `json:"length"`
	Points      json.RawMessage `json:"points"`
	Mask        json.RawMessage `json:"mask"`
	Closed      bool            `json:"closed"`
	Smoothing   float64         `json:"smoothing"`
	Threshold   float64         `json:"threshold"`
}

type track struct {
	Mode  string          `json:"mode"`
	Value float64         `json:"value"`
	Min   float64         `json:"min"`
	Max   float64         `json:"max"`
	Keys  []curveKey      `json:"keys"`
	A     json.RawMessage `json:"a"`
	B     json.RawMessage `json:"b"`
}

type curveKey struct {
	T    float64 `json:"t"`
	V    float64 `json:"v"`
	Ease string  `json:"ease"`
}

type gradientKey struct {
	T float64 `json:"t"`
	R float64 `json:"r"`
	G float64 `json:"g"`
	B float64 `json:"b"`
	A float64 `json:"a"`
}
