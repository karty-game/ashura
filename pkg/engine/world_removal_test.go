package engine

import (
	"slices"
	"strconv"
	"testing"
)

func TestWorldRemovalKeepsOrderBounded(t *testing.T) {
	t.Parallel()

	const count = 257
	for _, step := range []int{1, 73, count - 1} {
		t.Run(strconv.Itoa(step), func(t *testing.T) {
			t.Parallel()

			world := NewWorld()

			entities := make([]Entity, count)
			for index := range entities {
				entities[index] = world.Add(DefaultTransform())
			}

			for index := range count {
				entity := entities[index*step%count]
				if !world.Remove(entity) || world.Remove(entity) || world.Alive(entity) {
					t.Fatal("removal did not invalidate exactly one live handle")
				}

				if len(world.order) > 2*world.Len() {
					t.Fatal("removed identifiers grew beyond the live-order bound")
				}

				var previous uint32

				visited := 0

				world.Each(func(current Entity) {
					if !world.Alive(current) || current.index <= previous {
						t.Fatal("removed entity visited or creation order changed")
					}

					previous = current.index
					visited++
				})

				if visited != world.Len() {
					t.Fatal("removal skipped a surviving entity")
				}
			}

			if len(world.order) != 0 || world.next != count {
				t.Fatal("empty world retained ordered entries or reused identifiers")
			}
		})
	}
}

func TestWorldIterationSurvivesCompactionAndAddition(t *testing.T) {
	t.Parallel()

	for _, query := range []bool{false, true} {
		world := NewWorld()

		entities := make([]Entity, 5)
		for index := range entities {
			entities[index] = world.Add(DefaultTransform())
		}

		var added Entity

		visited := make([]Entity, 0, len(entities))

		visit := func(entity Entity) {
			visited = append(visited, entity)
			if entity == entities[1] {
				world.Remove(entities[0])
				world.Remove(entities[1])
				world.Remove(entities[3])

				added = world.Add(DefaultTransform())
			}
		}
		if query {
			for entity := range Query[Transform](world) {
				visit(entity)
			}
		} else {
			world.Each(visit)
		}

		want := []Entity{entities[0], entities[1], entities[2], entities[4], added}
		if !slices.Equal(visited, want) {
			t.Fatalf("query=%v: visits=%v, want %v", query, visited, want)
		}
	}
}

func BenchmarkWorldSceneRemoval(b *testing.B) {
	for _, count := range []int{1000, 5000, 20000} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			b.ReportAllocs()

			for range b.N {
				b.StopTimer()

				world := NewWorld()

				entities := make([]Entity, count)
				for index := range entities {
					entities[index] = world.Add(DefaultTransform())
				}

				b.StartTimer()

				for _, entity := range entities {
					if !world.Remove(entity) {
						b.Fatal("scene entity removal failed")
					}
				}
			}
		})
	}
}
