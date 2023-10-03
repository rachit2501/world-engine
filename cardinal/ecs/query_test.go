package ecs_test

import (
	"testing"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/filter"
	"pkg.world.dev/world-engine/cardinal/public"
)

func TestQueryEarlyTermination(t *testing.T) {
	type FooComponent struct {
		Data string
	}
	foo := ecs.NewComponentType[FooComponent]("foo")
	world := ecs.NewTestWorld(t)
	assert.NilError(t, world.RegisterComponents(foo))

	total := 10
	count := 0
	stop := 5
	_, err := world.CreateMany(total, foo)
	assert.NilError(t, err)
	ecs.NewQuery(filter.Exact(foo)).Each(world, func(id public.EntityID) bool {
		count++
		if count == stop {
			return false
		}
		return true
	})
	assert.Equal(t, count, stop)

	count = 0
	ecs.NewQuery(filter.Exact(foo)).Each(world, func(id public.EntityID) bool {
		count++
		return true
	})
	assert.Equal(t, count, total)
}
