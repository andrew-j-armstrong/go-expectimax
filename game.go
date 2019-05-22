package expectimax

import (
	"github.com/carbon-12/go-extensions"
)

type Game interface {
	IsGameOver() bool
	IsValidMove(interface{}) bool
	GetPossibleMoves() *extensions.InterfaceSlice
	MakeMove(interface{}) error
	Clone() interface{}
	RegisterMoveListener(chan<- interface{})
	Print()
}
