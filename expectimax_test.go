package expectimax

import (
	"testing"

	"github.com/carbon-12/go-extensions"
)

func TestGetBestMove(t *testing.T) {
	t.Run("test GetBestMove()", func(t *testing.T) {
		dummyMove := &struct{}{}
		expectimax := Expectimax{nil, nil, nil, nil, make(chan (chan<- interface{})), nil, 0}

		go func() {
			bestMoveChannel := <-expectimax.bestMoveChannelReceiver
			bestMoveChannel <- dummyMove
		}()

		if expectimax.GetBestMove() != dummyMove {
			t.Error("GetBestMove() failed to return expected move.")
		}
	})
}

func TestGetNextMoveValues(t *testing.T) {
	t.Run("test GetNextMoveValues()", func(t *testing.T) {
		dummyMap := extensions.ValueMap{}
		expectimax := Expectimax{nil, nil, nil, nil, nil, make(chan (chan<- *extensions.ValueMap)), 0}

		go func() {
			nextMoveChannel := <-expectimax.nextMoveChannelReceiver
			nextMoveChannel <- &dummyMap
		}()

		if expectimax.GetNextMoveValues() != &dummyMap {
			t.Error("GetNextMoveValues() failed to return expected move.")
		}
	})
}
