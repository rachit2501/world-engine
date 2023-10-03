package cardinal

import (
	"pkg.world.dev/world-engine/cardinal/ecs/read"
	"pkg.world.dev/world-engine/cardinal/public"
)

// AnyReadType is implemented by the return value of NewReadType and is used in RegisterReads; any
// read operation creates by NewReadType can be registered with a World object via RegisterReads.
type AnyReadType interface {
	Convert() public.IRead
}

// ReadType represents a read operation on a world object. The state of the world object must not be
// changed during the read operation.
type ReadType[Request, Reply any] struct {
	impl *read.ReadType[Request, Reply]
}

// NewReadType creates a new instance of a ReadType. The World state must not be changed
// in the given handler function.
func NewReadType[Request any, Reply any](
	name string,
	handler func(*World, Request) (Reply, error),
) *ReadType[Request, Reply] {
	return &ReadType[Request, Reply]{
		impl: read.NewReadType[Request, Reply](name, func(world public.IWorld, req Request) (Reply, error) {
			outerWorld := &World{implWorld: world}
			return handler(outerWorld, req)
		}),
	}
}

// NewReadTypeWithEVMSupport creates a new instance of a ReadType with EVM support, allowing this read to be called from
// the EVM base shard. The World state must not be changed in the given handler function.
func NewReadTypeWithEVMSupport[Request, Reply any](name string, handler func(*World, Request) (Reply, error)) *ReadType[Request, Reply] {
	return &ReadType[Request, Reply]{
		impl: read.NewReadType[Request, Reply](name, func(world public.IWorld, req Request) (Reply, error) {
			outerWorld := &World{implWorld: world}
			return handler(outerWorld, req)
		}, read.WithReadEVMSupport[Request, Reply]),
	}
}

// Convert implements the AnyReadType interface which allows a ReadType to be registered
// with a World via RegisterReads.
func (r *ReadType[Request, Reply]) Convert() public.IRead {
	return r.impl
}
