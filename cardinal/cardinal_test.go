package cardinal_test

import (
	"os"
	"os/exec"
	"strconv"
	"sync"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/gorilla/websocket"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/testutils"
)

type Foo struct{}

func (Foo) Name() string { return "foo" }

func TestNewWorld(t *testing.T) {
	// should fail, this test should generate a compile error if the function signature changes.
	_, err := cardinal.NewWorld("", "", cardinal.WithNamespace("testnamespace"))
	assert.Assert(t, err != nil)
}

func TestCanQueryInsideSystem(t *testing.T) {
	testutils.SetTestTimeout(t, 10*time.Second)

	world, doTick := testutils.MakeWorldAndTicker(t)
	assert.NilError(t, cardinal.RegisterComponent[Foo](world))

	wantNumOfEntities := 10
	world.Init(func(worldCtx cardinal.WorldContext) {
		_, err := cardinal.CreateMany(worldCtx, wantNumOfEntities, Foo{})
		assert.NilError(t, err)
	})
	gotNumOfEntities := 0
	cardinal.RegisterSystems(world, func(worldCtx cardinal.WorldContext) error {
		q, err := worldCtx.NewSearch(cardinal.Exact(Foo{}))
		assert.NilError(t, err)
		err = q.Each(worldCtx, func(cardinal.EntityID) bool {
			gotNumOfEntities++
			return true
		})
		assert.NilError(t, err)
		return nil
	})

	doTick()
	assert.Equal(t, world.CurrentTick(), uint64(1))
	err := world.ShutDown()
	assert.Assert(t, err)
	assert.Equal(t, gotNumOfEntities, wantNumOfEntities)
}

func TestShutdownViaSignal(t *testing.T) {
	// If this test is frozen then it failed to shut down, create a failure with panic.
	var wg sync.WaitGroup
	testutils.SetTestTimeout(t, 10*time.Second)
	world, err := cardinal.NewMockWorld()
	assert.NilError(t, cardinal.RegisterComponent[Foo](world))
	assert.NilError(t, err)
	wantNumOfEntities := 10
	world.Init(func(worldCtx cardinal.WorldContext) {
		_, err := cardinal.CreateMany(worldCtx, wantNumOfEntities, Foo{})
		assert.NilError(t, err)
	})
	wg.Add(1)
	go func() {
		err = world.StartGame()
		assert.NilError(t, err)
		wg.Done()
	}()
	for !world.IsGameRunning() {
		// wait until game loop is running
		time.Sleep(500 * time.Millisecond)
	}

	conn, _, err := websocket.DefaultDialer.Dial("ws://localhost:4040/events", nil)
	assert.NilError(t, err)
	wg.Add(1)
	go func() {
		_, _, err := conn.ReadMessage()
		assert.Assert(t, websocket.IsCloseError(err, websocket.CloseAbnormalClosure))
		wg.Done()
	}()
	// Send a SIGINT signal.
	cmd := exec.Command("kill", "-INT", strconv.Itoa(os.Getpid()))
	err = cmd.Run()
	assert.NilError(t, err)

	for world.IsGameRunning() {
		// wait until game loop is not running
		time.Sleep(500 * time.Millisecond)
	}
	wg.Wait()
}
