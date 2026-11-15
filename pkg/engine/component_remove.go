package engine

import "reflect"

// RemoveComponent removes exactly the stored value type T. It returns false for
// a missing component, removed/foreign entity or nil World. Removing an active
// material restores the most recently assigned remaining material provider;
// removing a camera projection/Transform refreshes camera eligibility immediately.
// Components that supply unrelated capabilities are never silently deleted.
func RemoveComponent[T any](world *World, entity Entity) bool {
	if !world.Alive(entity) {
		return false
	}

	kind := reflect.TypeFor[T]()
	if _, present := world.entities[entity][kind]; !present {
		return false
	}

	delete(world.entities[entity], kind)
	world.forgetComponent(entity, kind)
	world.refreshBindings(entity)

	return true
}
