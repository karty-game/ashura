//go:build !js

package main

import "log"

func attachTouchControls(_ string, _ *courtyardInput) (present func(trialStatus, bool), cleanup func()) {
	var instruction string

	return func(next trialStatus, _ bool) {
		if next.instruction == instruction {
			return
		}

		instruction = next.instruction
		log.Print(instruction)

		if next.finished {
			log.Printf("Time %s · best %s · press R for the other route", next.elapsed.Round(clockTick), next.best.Round(clockTick))
		}
	}, func() {}
}

func showCourtyardError(error) {}
