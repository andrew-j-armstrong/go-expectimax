package expectimax

import (
	"github.com/carbon-12/go-extensions"
)

type Game interface {
	IsGameOver() bool
	IsValidMoveGeneric(interface{}) bool
	GetPossibleMovesGeneric() *extensions.InterfaceSlice
	MakeMoveGeneric(interface{}) error
	CloneGeneric() interface{}
	RegisterMoveListenerGeneric(chan<- interface{})
	Print()
}
