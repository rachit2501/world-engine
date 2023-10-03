package storage_test

import (
	"testing"

	"pkg.world.dev/world-engine/cardinal/ecs/storage"
	"pkg.world.dev/world-engine/cardinal/interfaces"

	"pkg.world.dev/world-engine/cardinal/ecs/filter"
)

func TestIndex(t *testing.T) {
	var (
		ca = storage.NewMockComponentType(struct{}{}, nil)
		cb = storage.NewMockComponentType(struct{}{}, nil)
		cc = storage.NewMockComponentType(struct{}{}, nil)
	)

	index := storage.NewArchetypeComponentIndex()

	compsA := []interfaces.IComponentType{ca}
	compsB := []interfaces.IComponentType{ca, cb}

	index.Push(compsA)
	index.Push(compsB)

	tests := []struct {
		filter   interfaces.IComponentFilter
		expected int
	}{
		{

			filter:   filter.Contains(ca),
			expected: 2,
		},
		{

			filter:   filter.Contains(cb),
			expected: 1,
		},
		{

			filter:   filter.Contains(cc),
			expected: 0,
		},
	}

	for _, tt := range tests {
		it := index.Search(tt.filter)
		if len(it.GetValues()) != tt.expected {
			t.Errorf("Index should have %d archetypes", tt.expected)
		}
		if it.GetCurrent() != 0 && it.HasNext() {
			t.Errorf("Index should have 0 as current")
		}
		if tt.expected == 0 && it.HasNext() {
			t.Errorf("Index should not have next")
		}
	}
}
