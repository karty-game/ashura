package engine

import "iter"

// Query yields live entities containing T in creation order, with typed mutable
// component pointers. It accepts application-defined components. The iterator
// can be retained and reused; component admission/replacement is reflected on
// the next iteration. App/World mutation belongs to the App loop goroutine.
func Query[T any](world *World) iter.Seq2[Entity, *T] {
	return func(yield func(Entity, *T) bool) {
		if world == nil {
			return
		}

		for index := 0; index < len(world.order); {
			entity := world.order[index]
			if value, ok := Get[T](world, entity); ok && !yield(entity, value) {
				return
			}

			if index < len(world.order) && world.order[index] == entity {
				index++
			} else {
				index = world.afterEntity(entity)
			}
		}
	}
}

// Query2 yields entities containing both A and B without constructing results
// or exposing component storage. Removing an entity during iteration is safe.
func Query2[A, B any](world *World) iter.Seq2[Entity, Pair[A, B]] {
	return func(yield func(Entity, Pair[A, B]) bool) {
		for entity, a := range Query[A](world) {
			if b, ok := Get[B](world, entity); ok && !yield(entity, Pair[A, B]{First: a, Second: b}) {
				return
			}
		}
	}
}

// Pair contains typed component pointers for a Query2 result.
type Pair[A, B any] struct {
	First  *A
	Second *B
}
