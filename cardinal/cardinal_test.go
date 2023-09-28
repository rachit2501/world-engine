package cardinal_test

import (
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal"
)

// TODO this function needs to be moved to a utils package above cardinal to prevent circulars.
func setTestTimeout(t *testing.T, timeout time.Duration) {
	if _, ok := t.Deadline(); ok {
		// A deadline has already been set. Don't add an additional deadline.
		return
	}
	success := make(chan bool)
	t.Cleanup(func() {
		success <- true
	})
	go func() {
		select {
		case <-success:
			// test was successful. Do nothing
		case <-time.After(timeout):
			//assert.Check(t, false, "test timed out")
			panic("test timed out")
		}
	}()
}

func TestCanQueryInsideSystem(t *testing.T) {
	setTestTimeout(t, 10*time.Second)
	type Foo struct{}
	nextTickCh := make(chan time.Time)
	tickDoneCh := make(chan uint64)

	world, err := cardinal.NewMockWorld(
		cardinal.WithTickChannel(nextTickCh),
		cardinal.WithTickDoneChannel(tickDoneCh))
	assert.NilError(t, err)
	comp := cardinal.NewComponentType[Foo]("foo")
	world.RegisterComponents(comp)

	wantNumOfEntities := 10
	_, err = world.CreateMany(wantNumOfEntities, comp)
	assert.NilError(t, err)
	gotNumOfEntities := 0
	world.RegisterSystems(func(world *cardinal.World, queue *cardinal.TransactionQueue, logger *cardinal.Logger) error {
		cardinal.NewQuery(cardinal.Exact(comp)).Each(world, func(cardinal.EntityID) bool {
			gotNumOfEntities++
			return true
		})
		return nil
	})
	go func() {
		_ = world.StartGame()
	}()
	for !world.IsGameRunning() {
		time.Sleep(time.Second) //starting game async, must wait until game is running before testing everything.
	}
	nextTickCh <- time.Now()
	<-tickDoneCh
	err = world.ShutDown()
	assert.Assert(t, err)
	assert.Equal(t, gotNumOfEntities, wantNumOfEntities)
}
