//go:build ignore

package main

import (
	"github.com/karty-game/ashura/pkg/assetbuild"
	particlebuild "github.com/karty-game/ashura/pkg/particle/build"
)

const waterfallLandingOffset = -346

func main() {
	assetbuild.Run(assetbuild.Config{
		Output: "generated/assets",
		Pipelines: []assetbuild.Pipeline{{
			Name: "particles",
			Input: []string{
				"assets/solar-burst.prt",
				"assets/ion-plume.prt",
				"assets/hit-flash.prt",
				"assets/pickup-sparkle.prt",
				"assets/fireworks.prt",
				"assets/water-splash.prt",
				"assets/ice-shatter.prt",
				"assets/arcane-portal.prt",
				"assets/library/*.prt",
			},
			Output: []assetbuild.Plugin{particlebuild.Effects3D(map[string]particlebuild.SpatialLayers{
				"solar-burst": {
					"Copper cloud":         {Shape: particlebuild.Sphere, Radius: 24},
					"Fireball":             {Shape: particlebuild.Sphere, Radius: 14},
					"Expanding shock ring": {Shape: particlebuild.GroundPlane, HeightOffset: -114},
					"Hot fragments":        {Shape: particlebuild.Sphere, Radius: 15},
				},
				"ion-plume": {
					"Blue vapor":    {Shape: particlebuild.Cone, Radius: 20, SpreadDegrees: 72},
					"Cyan jet":      {Shape: particlebuild.Cone, Radius: 8, SpreadDegrees: 37},
					"Ion filaments": {Shape: particlebuild.Cone, Radius: 12, SpreadDegrees: 60},
				},
				"hit-flash": {
					"Impact shards": {Shape: particlebuild.Sphere, Radius: 12},
				},
				"pickup-sparkle": {
					"Collectible stars": {Shape: particlebuild.Sphere, Radius: 30},
				},
				"fireworks": {
					"Rocket":        {Shape: particlebuild.Cone},
					"Launch trail":  {Shape: particlebuild.Sphere, Radius: 2, RiseSpeed: 395, RiseGravity: 520},
					"Apex flash":    {Shape: particlebuild.Sphere, HeightOffset: 147},
					"Ruby bloom":    {Shape: particlebuild.Sphere, Radius: 5, HeightOffset: 147},
					"Gold sparks":   {Shape: particlebuild.Sphere, Radius: 5, HeightOffset: 147},
					"Violet embers": {Shape: particlebuild.Sphere, Radius: 6, HeightOffset: 147},
				},
				"water-splash": {
					"Splash mist":    {Shape: particlebuild.Cone, Radius: 18, SpreadDegrees: 142},
					"Blue droplets":  {Shape: particlebuild.Cone, Radius: 11, SpreadDegrees: 128},
					"Foam and spray": {Shape: particlebuild.Cone, Radius: 8, SpreadDegrees: 94},
					"Near ripple":    {Shape: particlebuild.GroundPlane, HeightOffset: -7, OffsetX: 4, OffsetZ: 62},
					"Left ripple":    {Shape: particlebuild.GroundPlane, HeightOffset: -7, OffsetX: -48, OffsetZ: 14},
					"Right ripple":   {Shape: particlebuild.GroundPlane, HeightOffset: -7, OffsetX: 45, OffsetZ: -22},
				},
				"ice-shatter": {
					"Frost mist":     {Shape: particlebuild.Sphere, Radius: 24},
					"Ice facets":     {Shape: particlebuild.Cone, Radius: 11, SpreadDegrees: 125},
					"Ice chips":      {Shape: particlebuild.Cone, Radius: 13, SpreadDegrees: 138},
					"Crystal glints": {Shape: particlebuild.Sphere, Radius: 16},
				},
				"arcane-portal": {
					"Portal interior": {Shape: particlebuild.Sphere, Radius: 39},
					"Rim sparks":      {Shape: particlebuild.Sphere, Radius: 89},
				},
				"blizzard": {
					"flakes-fast": {Shape: particlebuild.Cone, Radius: 210, SpreadDegrees: 24},
					"flakes-near": {Shape: particlebuild.Cone, Radius: 155, SpreadDegrees: 23},
					"gust-haze":   {Shape: particlebuild.Sphere, Radius: 150, HeightOffset: -105},
				},
				"bubbles": {
					"bubbles":    {Shape: particlebuild.Cone, Radius: 48, SpreadDegrees: 27},
					"micro-fizz": {Shape: particlebuild.Cone, Radius: 43, SpreadDegrees: 38},
				},
				"button-pop": {
					"sparks": {Shape: particlebuild.Sphere, Radius: 13},
					"dots":   {Shape: particlebuild.Sphere, Radius: 10},
				},
				"cinder-fall": {
					"cinders": {Shape: particlebuild.Cone, Radius: 15, SpreadDegrees: 50},
					"warmth":  {Shape: particlebuild.Sphere, Radius: 93},
				},
				"confetti": {
					"c1": {Shape: particlebuild.Cone, Radius: 83, SpreadDegrees: 52},
					"c2": {Shape: particlebuild.Cone, Radius: 83, SpreadDegrees: 52},
					"c3": {Shape: particlebuild.Cone, Radius: 83, SpreadDegrees: 52},
					"c4": {Shape: particlebuild.Cone, Radius: 83, SpreadDegrees: 52},
				},
				"dust-puff": {
					"puff": {Shape: particlebuild.Sphere, Radius: 12},
					"grit": {Shape: particlebuild.Cone, Radius: 10, SpreadDegrees: 145},
				},
				"fire": {
					"Soot":          {Shape: particlebuild.Cone, Radius: 19, SpreadDegrees: 34, HeightOffset: 69},
					"Flame base":    {Shape: particlebuild.Cone, Radius: 23, SpreadDegrees: 34},
					"Flame body":    {Shape: particlebuild.Cone, Radius: 21, SpreadDegrees: 19},
					"Flame tongues": {Shape: particlebuild.Cone, Radius: 16, SpreadDegrees: 20},
					"Rising embers": {Shape: particlebuild.Cone, Radius: 16, SpreadDegrees: 48},
				},
				"fog-bank": {
					"fog-near": {Shape: particlebuild.Sphere, Radius: 150},
					"fog-far":  {Shape: particlebuild.Sphere, Radius: 155, HeightOffset: 22},
					"wisps":    {Shape: particlebuild.Sphere, Radius: 130},
				},
				"geyser": {
					"column":   {Shape: particlebuild.Cone, Radius: 10, SpreadDegrees: 25},
					"droplets": {Shape: particlebuild.Cone, Radius: 12, SpreadDegrees: 77},
					"mist":     {Shape: particlebuild.Cone, Radius: 16, SpreadDegrees: 110},
				},
				"healing-aura": {
					"motes-rising": {Shape: particlebuild.Cone, Radius: 37, SpreadDegrees: 22},
					"plus-glints":  {Shape: particlebuild.Cone, Radius: 33, SpreadDegrees: 25},
				},
				"hit-spark-star": {
					"spikes": {Shape: particlebuild.Sphere, Radius: 6},
					"debris": {Shape: particlebuild.Sphere, Radius: 7},
				},
				"laser-impact": {
					"plasma":       {Shape: particlebuild.Cone, Radius: 8, SpreadDegrees: 100},
					"scorch-smoke": {Shape: particlebuild.Sphere, Radius: 8},
				},
				"level-up-burst": {
					"ring":  {Shape: particlebuild.GroundPlane, HeightOffset: -24},
					"motes": {Shape: particlebuild.Sphere, Radius: 55},
				},
				"magic-burst": {
					"swirl": {Shape: particlebuild.Sphere, Radius: 10},
					"motes": {Shape: particlebuild.Sphere, Radius: 45},
				},
				"poison-cloud": {
					"haze":    {Shape: particlebuild.Sphere, Radius: 67},
					"murk":    {Shape: particlebuild.Sphere, Radius: 57},
					"bubbles": {Shape: particlebuild.Sphere, Radius: 53},
					"pops":    {Shape: particlebuild.Sphere, Radius: 62},
				},
				"rocket-exhaust": {
					"flame":       {Shape: particlebuild.Cone, Radius: 5, SpreadDegrees: 18},
					"smoke-trail": {Shape: particlebuild.Cone, Radius: 9, SpreadDegrees: 45},
					"embers":      {Shape: particlebuild.Cone, Radius: 6, SpreadDegrees: 43},
				},
				"shockwave-ring": {
					"annulus-haze": {Shape: particlebuild.GroundPlane},
					"ring":         {Shape: particlebuild.GroundPlane},
				},
				"snow": {
					"snow-far":  {Shape: particlebuild.Cone, Radius: 180, SpreadDegrees: 32},
					"snow-near": {Shape: particlebuild.Cone, Radius: 130, SpreadDegrees: 35},
				},
				"torch-flame": {
					"Heat glow":  {Shape: particlebuild.Cone, Radius: 4, SpreadDegrees: 17},
					"Flame":      {Shape: particlebuild.Cone, Radius: 6, SpreadDegrees: 13},
					"Flame tips": {Shape: particlebuild.Cone, Radius: 6, SpreadDegrees: 13},
					"Wick":       {Shape: particlebuild.Cone, Radius: 3, SpreadDegrees: 10},
					"Sparks":     {Shape: particlebuild.Cone, Radius: 4, SpreadDegrees: 35},
				},
				"varied-embers": {
					"embers": {Shape: particlebuild.Cone, Radius: 97, SpreadDegrees: 47},
					"warmth": {Shape: particlebuild.Sphere, Radius: 83},
				},
				"waterfall-mist": {
					"sheet":        {Shape: particlebuild.Cone, Radius: 36, SpreadDegrees: 6},
					"impact spray": {Shape: particlebuild.Cone, Radius: 29, SpreadDegrees: 135, HeightOffset: waterfallLandingOffset},
					"mist":         {Shape: particlebuild.Cone, Radius: 39, SpreadDegrees: 134, HeightOffset: waterfallLandingOffset},
				},
			})},
		}},
	})
}
