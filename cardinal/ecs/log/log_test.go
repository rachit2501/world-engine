package log_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/component/metadata"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/log"
)

type SendEnergyTx struct {
	From, To string
	Amount   uint64
}

type SendEnergyTxResult struct{}

type EnergyComp struct {
	value int
}

func (EnergyComp) Name() string {
	return "EnergyComp"
}

func testSystem(wCtx ecs.WorldContext) error {
	wCtx.Logger().Log().Msg("test")
	q, err := wCtx.NewSearch(ecs.Contains(EnergyComp{}))
	if err != nil {
		return err
	}
	err = q.Each(wCtx, func(entityId entity.ID) bool {
		energyPlanet, err := component.GetComponent[EnergyComp](wCtx, entityId)
		if err != nil {
			return false
		}
		energyPlanet.value += 10
		err = component.SetComponent[EnergyComp](wCtx, entityId, energyPlanet)
		return err == nil
	})
	if err != nil {
		panic(err)
	}

	return nil
}

func testSystemWarningTrigger(wCtx ecs.WorldContext) error {
	time.Sleep(time.Millisecond * 400)
	return testSystem(wCtx)
}

func TestWorldLogger(t *testing.T) {
	w := ecs.NewTestWorld(t)
	// replaces internal Logger with one that logs to the buf variable above.
	var buf bytes.Buffer
	bufLogger := zerolog.New(&buf)
	cardinalLogger := log.Logger{
		&bufLogger,
	}
	w.InjectLogger(&cardinalLogger)
	alphaTx := ecs.NewTransactionType[SendEnergyTx, SendEnergyTxResult]("alpha")
	assert.NilError(t, w.RegisterTransactions(alphaTx))
	assert.NilError(t, ecs.RegisterComponent[EnergyComp](w))
	cardinalLogger.LogWorld(w, zerolog.InfoLevel)
	jsonWorldInfoString := `{
					"level":"info",
					"total_components":2,
					"components":
						[
							{
								"component_id":1,
								"component_name":"SignerComponent"
							},
							{
								"component_id":2,
								"component_name":"EnergyComp"
							}
						],
					"total_systems":2,
					"systems":
						[
							"ecs.RegisterPersonaSystem",
							"ecs.AuthorizePersonaAddressSystem"
						]
				}
`
	require.JSONEq(t, jsonWorldInfoString, buf.String())
	buf.Reset()
	energy, err := w.GetComponentByName(EnergyComp{}.Name())
	assert.NilError(t, err)
	components := []metadata.ComponentMetadata{energy}
	wCtx := ecs.NewWorldContext(w)
	entityID, err := component.Create(wCtx, EnergyComp{})
	assert.NilError(t, err)
	logStrings := strings.Split(buf.String(), "\n")[:2]
	require.JSONEq(t, `
			{
				"level":"debug",
				"archetype_id":0,
				"message":"created"
			}`, logStrings[0])
	require.JSONEq(t, `
			{
				"level":"debug",
				"components":[{
					"component_id":2,
					"component_name":"EnergyComp"
				}],
				"entity_id":0,"archetype_id":0
			}`, logStrings[1])

	buf.Reset()

	// test log entity
	archetypeID, err := w.StoreManager().GetArchIDForComponents(components)
	assert.NilError(t, err)
	cardinalLogger.LogEntity(zerolog.DebugLevel, entityID, archetypeID, components)
	jsonEntityInfoString := `
		{
			"level":"debug",
			"components":[
				{
					"component_id":2,
					"component_name":"EnergyComp"
				}],
			"entity_id":0,
			"archetype_id":0
		}`
	require.JSONEq(t, buf.String(), jsonEntityInfoString)

	// create a system for logging.
	buf.Reset()
	w.AddSystems(testSystemWarningTrigger)
	err = w.LoadGameState()
	assert.NilError(t, err)
	ctx := context.Background()

	// testing output of logging a tick. Should log the system log and tick start and end strings.
	err = w.Tick(ctx)
	assert.NilError(t, err)
	logStrings = strings.Split(buf.String(), "\n")[:4]
	// test tick start
	require.JSONEq(t, `
			{
				"level":"info",
				"tick":"0",
				"message":"Tick started"
			}`, logStrings[0])
	// test if system name recorded in log
	require.JSONEq(t, `
			{
				"system":"log_test.testSystemWarningTrigger",
				"message":"test"
			}`, logStrings[1])
	// test if updating component worked
	require.JSONEq(t, `
			{
				"level":"debug",
				"entity_id":"0",
				"component_name":"EnergyComp",
				"component_id":2,
				"message":"entity updated",
				"system":"log_test.testSystemWarningTrigger"
			}`, logStrings[2])
	// test tick end
	buf.Reset()
	sanitizedJSON := func(json []byte) []byte {
		json = bytes.ReplaceAll(json, []byte("\t"), []byte(""))
		json = bytes.ReplaceAll(json, []byte("\n"), []byte(""))
		json = bytes.ReplaceAll(json, []byte("\r"), []byte(""))
		return json
	}
	var map1, map2 map[string]interface{}
	// tick execution time is not tested.
	json1 := []byte(`{
				 "level":"warn",
				 "tick":"0",
				 "tick_execution_time": 0, 
				 "message":"tick ended, (warning: tick exceeded 100ms)"
			 }`)
	json1 = sanitizedJSON(json1)
	if err = json.Unmarshal(json1, &map1); err != nil {
		t.Fatalf("Error unmarshalling json1: %v", err)
	}
	if err = json.Unmarshal([]byte(logStrings[3]), &map2); err != nil {
		t.Fatalf("Error unmarshalling buf: %v", err)
	}
	names := []string{"level", "tick", "tick_execution_time", "message"}
	for _, name := range names {
		v1, ok := map1[name]
		if !ok {
			t.Errorf("Should be a value in %s", name)
		}
		v2, ok := map2[name]
		if !ok {
			t.Errorf("Should be a value in %s", name)
		}
		// time is not deterministic in the context of unit tests, therefore it is not unit testable.
		if name != "tick_execution_time" {
			assert.Equal(t, v1, v2)
		}
	}

	// testing log output for the creation of two entities.
	buf.Reset()
	_, err = component.CreateMany(wCtx, 2, EnergyComp{})
	assert.NilError(t, err)
	entityCreationStrings := strings.Split(buf.String(), "\n")[:2]
	require.JSONEq(t, `
			{
				"level":"debug",
				"components":
					[
						{
							"component_id":2,
							"component_name":"EnergyComp"
						}
					],
				"entity_id":1,
				"archetype_id":0
			}`, entityCreationStrings[0])
	require.JSONEq(t, `
			{
				"level":"debug",
				"components":
					[
						{
							"component_id":2,
							"component_name":"EnergyComp"
						}
					],
				"entity_id":2,
				"archetype_id":0
			}`, entityCreationStrings[1])
}
