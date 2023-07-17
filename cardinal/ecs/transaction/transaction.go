package transaction

type TypeID int

type ITransaction interface {
	SetID(TypeID) error
	Name() string
	ID() TypeID
	Encode(any) ([]byte, error)
	Decode([]byte) (any, error)
}
