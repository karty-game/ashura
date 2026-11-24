package engine

import (
	"errors"
	"math"
	"reflect"
	"sort"
)

var errEntityLimit = errors.New("world lifetime entity limit reached")

var errInvalidEntity = ErrInvalidEntity

var errMissingCamera = errors.New("entity has no camera projection component")

var errMissingCameraTransform = errors.New("camera entity has no transform component")

// Entity is an opaque, comparable handle owned by one World.
type Entity struct {
	world *World
	index uint32
}

// IsZero reports whether the handle is the zero value.
func (entity Entity) IsZero() bool { return entity.world == nil }

// World stores entities and their components. Components are attached by value
// and may be read or changed through typed accessors.
type World struct {
	next           uint32
	active         Entity
	entities       map[Entity]map[reflect.Type]any
	componentOrder map[Entity][]reflect.Type
	geometry       map[meshGeometryKey]*renderGeometry
	order          []Entity
	cameras        []Entity
	frame          frameStorage
	bindings       map[Entity]*entityBindings
	decals         *worldDecals
}

// NewWorld creates an empty ECS world.
func NewWorld() *World {
	return &World{entities: make(map[Entity]map[reflect.Type]any)}
}

// Add creates an entity and attaches the supplied component values. If the
// same component type appears more than once, the last value is retained.
// The most recently assigned material provider is active; inactive components
// remain available through Get and typed queries until explicitly removed.
func (world *World) Add(components ...any) Entity {
	if world.next == math.MaxUint32 {
		panic(errEntityLimit)
	}

	world.next++
	entity := Entity{world: world, index: world.next}
	world.entities[entity] = make(map[reflect.Type]any, len(components))
	world.order = append(world.order, entity)

	for _, component := range components {
		world.put(entity, component)
	}

	world.refreshBindings(entity)

	return entity
}

// Set adds or replaces a component on an existing entity. Assignment makes
// that component the most recent provider for its capabilities, including the
// active material; other component types remain attached.
// A nil component (including a typed nil pointer) returns ErrInvalidComponent.
func (world *World) Set(entity Entity, component any) error {
	if !world.Alive(entity) {
		return errInvalidEntity
	}

	if !world.put(entity, component) {
		return ErrInvalidComponent
	}

	world.refreshBindings(entity)

	return nil
}

// Get returns a pointer to an entity's component value.
func Get[T any](world *World, entity Entity) (*T, bool) {
	if world == nil || !world.Alive(entity) {
		return nil, false
	}

	stored, found := world.entities[entity][reflect.TypeFor[T]()]
	if !found {
		return nil, false
	}

	value, ok := stored.(*T)

	return value, ok
}

// Transform returns the mutable Transform component, if present.
func (world *World) Transform(entity Entity) *Transform {
	if world == nil {
		return nil
	}

	if entry := world.bindings[entity]; entry != nil {
		return entry.transform
	}

	return nil
}

// Alive reports whether entity is still alive in this World.
func (world *World) Alive(entity Entity) bool {
	if world == nil || entity.world != world || entity.index == 0 {
		return false
	}

	_, ok := world.entities[entity]

	return ok
}

// Remove destroys an entity and all its components.
func (world *World) Remove(entity Entity) bool {
	if !world.Alive(entity) {
		return false
	}

	delete(world.entities, entity)
	delete(world.componentOrder, entity)

	if entry := world.bindings[entity]; entry != nil && entry.decalProjector != nil {
		world.bindDecal(entity, nil)
	}

	delete(world.bindings, entity)
	world.updateCameraBinding(entity, nil)
	delete(world.frame.batches, entity.index)

	index := sort.Search(len(world.order), func(index int) bool { return world.order[index].index >= entity.index })
	// Keep the identifier sorted so an iterator can resume after removal or
	// compaction without shifting the surviving tail for every entity removed.
	world.order[index].world = nil
	if len(world.order)-len(world.entities) > len(world.entities) {
		world.compactOrder()
	}

	if world.active == entity {
		world.active = Entity{}
	}

	return true
}

// Len returns the number of live entities.
func (world *World) Len() int { return len(world.entities) }

// SetActiveCamera selects an entity for rendering as the world's camera.
// The entity must be alive and have a camera projection component.
func (world *World) SetActiveCamera(entity Entity) error {
	if !world.Alive(entity) {
		return errInvalidEntity
	}

	if _, found := projectionForEntity(world, entity); !found {
		return errMissingCamera
	}

	if _, hasTransform := Get[Transform](world, entity); !hasTransform {
		return errMissingCameraTransform
	}

	world.active = entity

	return nil
}

// ActiveCamera returns the selected live camera, if one has been set.
func (world *World) ActiveCamera() (Entity, bool) {
	if !world.Alive(world.active) {
		return Entity{}, false
	}

	return world.active, true
}

// Each visits live entities in creation order. Removing the current entity or
// earlier entities during a callback does not skip surviving later entities.
func (world *World) Each(visit func(Entity)) {
	if world == nil || visit == nil {
		return
	}

	for index := 0; index < len(world.order); {
		entity := world.order[index]
		if entity.world != nil {
			visit(entity)
		}

		if index < len(world.order) && world.order[index] == entity {
			index++
		} else {
			index = world.afterEntity(entity)
		}
	}
}

func (world *World) eachComponent(entity Entity, visit func(any)) {
	if world == nil || visit == nil || !world.Alive(entity) {
		return
	}

	for _, kind := range world.componentOrder[entity] {
		visit(world.entities[entity][kind])
	}
}

func (world *World) put(entity Entity, component any) bool {
	if component == nil {
		return false
	}

	value := reflect.ValueOf(component)
	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return false
		}

		value = value.Elem()
	}

	componentType := value.Type()
	copyOfComponent := reflect.New(componentType)
	copyOfComponent.Elem().Set(value)
	world.entities[entity][componentType] = copyOfComponent.Interface()
	world.touchComponent(entity, componentType)

	return true
}

func (world *World) afterEntity(entity Entity) int {
	return sort.Search(len(world.order), func(index int) bool { return world.order[index].index > entity.index })
}

// Compaction runs only at structural deletion and its total work is amortized
// over preceding removals. Slots retain at most one dead identifier per survivor.
func (world *World) compactOrder() {
	live := 0

	for _, entity := range world.order {
		if entity.world != nil {
			world.order[live] = entity
			live++
		}
	}

	clear(world.order[live:])
	world.order = world.order[:live]
}

func (world *World) touchComponent(entity Entity, kind reflect.Type) {
	if world.componentOrder == nil {
		world.componentOrder = make(map[Entity][]reflect.Type)
	}

	world.forgetComponent(entity, kind)
	world.componentOrder[entity] = append(world.componentOrder[entity], kind)
}

func (world *World) forgetComponent(entity Entity, kind reflect.Type) {
	order := world.componentOrder[entity]
	for index, current := range order {
		if current == kind {
			copy(order[index:], order[index+1:])
			order[len(order)-1] = nil
			world.componentOrder[entity] = order[:len(order)-1]

			return
		}
	}
}
