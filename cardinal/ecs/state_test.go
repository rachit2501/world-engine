package ecs_test

import (
	"context"
	"pkg.world.dev/world-engine/cardinal/ecs/internal/testutil"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/inmem"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
)

// comps reduces the typing needed to create a slice of IComponentTypes
// []component.IComponentType{a, b, c} becomes:
// comps(a, b, c)
func comps(cs ...component.IComponentType) []component.IComponentType {
	return cs
}

type NumberComponent struct {
	Num int
}

func TestComponentsCanOnlyBeRegisteredOnce(t *testing.T) {
	world := inmem.NewECSWorldForTest(t)
	assert.NilError(t, world.RegisterComponents())
	assert.ErrorIs(t, world.RegisterComponents(), ecs.ErrorComponentRegistrationMustHappenOnce)
}

func TestErrorWhenSavedArchetypesDoNotMatchComponentTypes(t *testing.T) {
	// This redisStore will be used to create multiple worlds to ensure state is consistent across the worlds.
	redisStore := miniredis.RunT(t)

	oneWorld := testutil.InitWorldWithRedis(t, redisStore)
	oneAlphaNum := ecs.NewComponentType[NumberComponent]()
	assert.NilError(t, oneWorld.RegisterComponents(oneAlphaNum))
	assert.NilError(t, oneWorld.LoadGameState())

	_, err := oneWorld.Create(oneAlphaNum)
	assert.NilError(t, err)

	assert.NilError(t, oneWorld.Tick(context.Background()))

	// Too few components registered
	twoWorld := testutil.InitWorldWithRedis(t, redisStore)
	assert.NilError(t, twoWorld.RegisterComponents())
	err = twoWorld.LoadGameState()
	assert.ErrorContains(t, err, storage.ErrorComponentMismatchWithSavedState.Error())

	// It's ok to register extra components.
	threeWorld := testutil.InitWorldWithRedis(t, redisStore)
	threeAlphaNum := ecs.NewComponentType[NumberComponent]()
	threeBetaNum := ecs.NewComponentType[NumberComponent]()
	assert.NilError(t, threeWorld.RegisterComponents(threeAlphaNum, threeBetaNum))
	assert.NilError(t, threeWorld.LoadGameState())

	// Just the right number of components registered
	fourWorld := testutil.InitWorldWithRedis(t, redisStore)
	fourAlphaNum := ecs.NewComponentType[NumberComponent]()
	assert.NilError(t, fourWorld.RegisterComponents(fourAlphaNum))
	assert.NilError(t, fourWorld.LoadGameState())
}

func TestArchetypeIDIsConsistentAfterSaveAndLoad(t *testing.T) {
	redisStore := miniredis.RunT(t)
	oneWorld := testutil.InitWorldWithRedis(t, redisStore)
	oneNum := ecs.NewComponentType[NumberComponent]()
	assert.NilError(t, oneWorld.RegisterComponents(oneNum))
	assert.NilError(t, oneWorld.LoadGameState())

	_, err := oneWorld.Create(oneNum)
	assert.NilError(t, err)

	wantID := oneWorld.GetArchetypeForComponents(comps(oneNum))
	wantLayout := oneWorld.Archetype(wantID).Layout()
	assert.Equal(t, 1, len(wantLayout.Components()))
	assert.Check(t, wantLayout.HasComponent(oneNum))

	assert.NilError(t, oneWorld.Tick(context.Background()))

	// Make a second instance of the world using the same storage.
	twoWorld := testutil.InitWorldWithRedis(t, redisStore)
	twoNum := ecs.NewComponentType[NumberComponent]()
	assert.NilError(t, twoWorld.RegisterComponents(twoNum))
	assert.NilError(t, twoWorld.LoadGameState())

	gotID := twoWorld.GetArchetypeForComponents(comps(twoNum))
	gotLayout := twoWorld.Archetype(gotID).Layout()
	assert.Equal(t, 1, len(gotLayout.Components()))
	assert.Check(t, gotLayout.HasComponent(twoNum))

	// Archetype indices should be the same across save/load cycles
	assert.Equal(t, wantID, gotID)
}

func TestCanRecoverArchetypeInformationAfterLoad(t *testing.T) {
	redisStore := miniredis.RunT(t)

	oneWorld := testutil.InitWorldWithRedis(t, redisStore)
	oneAlphaNum := ecs.NewComponentType[NumberComponent]()
	oneBetaNum := ecs.NewComponentType[NumberComponent]()
	assert.NilError(t, oneWorld.RegisterComponents(oneAlphaNum, oneBetaNum))
	assert.NilError(t, oneWorld.LoadGameState())

	_, err := oneWorld.Create(oneAlphaNum)
	assert.NilError(t, err)
	_, err = oneWorld.Create(oneBetaNum)
	assert.NilError(t, err)
	_, err = oneWorld.Create(oneAlphaNum, oneBetaNum)
	assert.NilError(t, err)
	// At this point 3 archetypes exist:
	// oneAlphaNum
	// oneBetaNum
	// oneAlphaNum, oneBetaNum
	oneJustAlphaArchID := oneWorld.GetArchetypeForComponents(comps(oneAlphaNum))
	oneJustBetaArchID := oneWorld.GetArchetypeForComponents(comps(oneBetaNum))
	oneBothArchID := oneWorld.GetArchetypeForComponents(comps(oneAlphaNum, oneBetaNum))
	// These archetype indices should be preserved between a state save/load

	assert.NilError(t, oneWorld.Tick(context.Background()))

	// Create a brand new world, but use the original redis store. We should be able to load
	// the game state from the redis store (including archetype indices).
	twoWorld := testutil.InitWorldWithRedis(t, redisStore)
	twoAlphaNum := ecs.NewComponentType[NumberComponent]()
	twoBetaNum := ecs.NewComponentType[NumberComponent]()
	// The ordering of registering these components is important. It must match the ordering above.
	assert.NilError(t, twoWorld.RegisterComponents(twoAlphaNum, twoBetaNum))
	assert.NilError(t, twoWorld.LoadGameState())

	// Don't create any entities like above; they should already exist

	// The order that we FETCH archetypes shouldn't matter, so this order is intentionally
	// different from the setup step
	twoBothArchID := oneWorld.GetArchetypeForComponents(comps(oneBetaNum, oneAlphaNum))
	assert.Equal(t, oneBothArchID, twoBothArchID)
	twoJustAlphaArchID := oneWorld.GetArchetypeForComponents(comps(oneAlphaNum))
	assert.Equal(t, oneJustAlphaArchID, twoJustAlphaArchID)
	twoJustBetaArchID := oneWorld.GetArchetypeForComponents(comps(oneBetaNum))
	assert.Equal(t, oneJustBetaArchID, twoJustBetaArchID)

	// Save and load again to make sure the "two" world correctly saves its state even though
	// it never created any entities
	assert.NilError(t, twoWorld.Tick(context.Background()))

	threeWorld := testutil.InitWorldWithRedis(t, redisStore)
	threeAlphaNum := ecs.NewComponentType[NumberComponent]()
	threeBetaNum := ecs.NewComponentType[NumberComponent]()
	// Again, the ordering of registering these components is important. It must match the ordering above
	assert.NilError(t, threeWorld.RegisterComponents(threeAlphaNum, threeBetaNum))
	assert.NilError(t, threeWorld.LoadGameState())

	// And again, the loading of archetypes is intentionally different from the above two steps
	threeJustBetaArchID := oneWorld.GetArchetypeForComponents(comps(oneBetaNum))
	assert.Equal(t, oneJustBetaArchID, threeJustBetaArchID)
	threeBothArchID := oneWorld.GetArchetypeForComponents(comps(oneBetaNum, oneAlphaNum))
	assert.Equal(t, oneBothArchID, threeBothArchID)
	threeJustAlphaArchID := oneWorld.GetArchetypeForComponents(comps(oneAlphaNum))
	assert.Equal(t, oneJustAlphaArchID, threeJustAlphaArchID)
}

func TestCanReloadState(t *testing.T) {
	redisStore := miniredis.RunT(t)
	alphaWorld := testutil.InitWorldWithRedis(t, redisStore)
	oneAlphaNum := ecs.NewComponentType[NumberComponent]()
	assert.NilError(t, alphaWorld.RegisterComponents(oneAlphaNum))

	_, err := alphaWorld.CreateMany(10, oneAlphaNum)
	assert.NilError(t, err)
	alphaWorld.AddSystem(func(w *ecs.World, queue *ecs.TransactionQueue) error {
		oneAlphaNum.Each(w, func(id storage.EntityID) bool {
			err := oneAlphaNum.Set(w, id, NumberComponent{int(id)})
			assert.Check(t, err == nil)
			return true
		})
		return nil
	})
	assert.NilError(t, alphaWorld.LoadGameState())

	// Start a tick with executes the above system which initializes the number components.
	assert.NilError(t, alphaWorld.Tick(context.Background()))

	// Make a new world, using the original redis DB that (hopefully) has our data
	betaWorld := testutil.InitWorldWithRedis(t, redisStore)
	oneBetaNum := ecs.NewComponentType[NumberComponent]()
	assert.NilError(t, betaWorld.RegisterComponents(oneBetaNum))
	assert.NilError(t, betaWorld.LoadGameState())

	count := 0
	oneBetaNum.Each(betaWorld, func(id storage.EntityID) bool {
		count++
		num, err := oneBetaNum.Get(betaWorld, id)
		assert.NilError(t, err)
		assert.Equal(t, int(id), num.Num)
		return true
	})
	// Make sure we actually have 10 entities
	assert.Equal(t, 10, count)
}