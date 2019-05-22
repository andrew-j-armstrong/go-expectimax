package expectimax

import (
	"fmt"
	"log"
	"time"

	"github.com/carbon-12/go-extensions"
)

type ExpectimaxHeuristic func(game Game) float64

type ExpectimaxChildLikelihoodFunc func(getGame func() Game, getChildValue func(interface{}) float64, childLikelihood *extensions.ValueMap)

type Expectimax struct {
	game                          Game // Current game state
	heuristic                     ExpectimaxHeuristic
	calculateChildLikelihood      ExpectimaxChildLikelihoodFunc
	rootNode                      *expectimaxNode
	bestMoveChannelReceiver       chan (chan<- interface{})
	nextMoveChannelReceiver       chan (chan<- *extensions.ValueMap)
	unexploredNodeReceiverChannel chan chan<- *expectimaxNode
	exploredNodeChannel           chan *expectimaxNode
	maxNodeCount                  int
}

func (this *Expectimax) GetBestMove() interface{} {
	bestMoveChannel := make(chan interface{})

	this.bestMoveChannelReceiver <- bestMoveChannel

	return <-bestMoveChannel
}

func (this *Expectimax) GetNextMoveValues() *extensions.ValueMap {
	nextMoveValuesChannel := make(chan *extensions.ValueMap)

	this.nextMoveChannelReceiver <- nextMoveValuesChannel

	return <-nextMoveValuesChannel
}

func (this *Expectimax) IsCurrentlySearching() bool {
	if this.rootNode == nil {
		return false
	}

	return this.rootNode.descendentCount < this.maxNodeCount &&
		(this.rootNode.mostLikelyUnexploredDescendent != nil ||
			len(this.unexploredNodeReceiverChannel) != expectimaxWorkerCount ||
			len(this.exploredNodeChannel) != 0)
}

func (this *Expectimax) sendBestMove(bestMoveChannel chan<- interface{}) {
	if this.rootNode.descendentCount < this.maxNodeCount/100 && this.rootNode.mostLikelyUnexploredDescendent != nil {
		// Wait for more depth to be explored
		go func() {
			time.Sleep(time.Duration(100) * time.Millisecond)
			this.bestMoveChannelReceiver <- bestMoveChannel
		}()
	} else {
		var bestChildMove interface{}
		var bestChildValue float64
		for childMove, childNode := range this.rootNode.children {
			if bestChildMove == nil || bestChildValue < childNode.value {
				bestChildMove = childMove
				bestChildValue = childNode.value
			}
		}

		bestMoveChannel <- bestChildMove
	}
}

const expectimaxWorkerCount int = 10

func (this *Expectimax) RunExpectimax() {
	this.rootNode = NewBaseNode(this.game)

	moveListener := make(chan interface{}, 4)
	this.game.RegisterMoveListener(moveListener)

	this.unexploredNodeReceiverChannel = make(chan chan<- *expectimaxNode, expectimaxWorkerCount)
	this.exploredNodeChannel = make(chan *expectimaxNode, 10*expectimaxWorkerCount)
	exploreNodeWorkers := make([]*exploreNodeWorker, 0, expectimaxWorkerCount)

	for i := 0; i < expectimaxWorkerCount; i++ {
		exploreNodeWorker := NewExploreNodeWorker(this.unexploredNodeReceiverChannel, this.exploredNodeChannel)
		exploreNodeWorkers = append(exploreNodeWorkers, exploreNodeWorker)
		go exploreNodeWorker.ExploreNodeThread(this.heuristic, this.calculateChildLikelihood)
	}

	exploreNodeCount := 0
	go func() {
		lastExploreCount := 0
		for {
			time.Sleep(time.Second)
			if exploreNodeCount != 0 || lastExploreCount != 0 {
				fmt.Printf("Explore Count: %d. Waiting workers: %d. Allocated nodes: %d. Expected result: %g\n", exploreNodeCount, len(this.unexploredNodeReceiverChannel), this.rootNode.descendentCount, this.rootNode.value)
			}
			lastExploreCount = exploreNodeCount
			exploreNodeCount = 0
		}
	}()

	for {
		select {
		case move := <-moveListener:
			if move == nil {
				break
			}

			// Ensure rootNode has been explored
			switch this.rootNode.explorationStatus {
			case Unexplored:
				// Unexplored and not waiting for exploration, so just explore it now
				this.rootNode.Explore(this.heuristic, this.calculateChildLikelihood)
				this.rootNode.processExploredNode(this.calculateChildLikelihood)
			case WaitingForExploration, Exploring:
				for this.rootNode.explorationStatus != Archived {
					exploredNode := <-this.exploredNodeChannel
					exploredNode.processExploredNode(this.calculateChildLikelihood)
					exploredNode.decrementReference()
				}
			}

			this.rootNode = this.rootNode.descendToChild(move)

			if this.rootNode.game.IsGameOver() {
				break
			}

		case exploredNode := <-this.exploredNodeChannel:
			exploreNodeCount++
			exploredNode.processExploredNode(this.calculateChildLikelihood)
			go exploredNode.decrementReference()

		case bestMoveChannel := <-this.bestMoveChannelReceiver:
			if len(moveListener) > 0 {
				// If there are moves to be processed, do those first
				this.bestMoveChannelReceiver <- bestMoveChannel
				break
			}

			this.sendBestMove(bestMoveChannel)

		case nextMoveChannel := <-this.nextMoveChannelReceiver:
			if len(moveListener) > 0 {
				// If there are moves to be processed, do those first
				this.nextMoveChannelReceiver <- nextMoveChannel
				break
			}

			if this.rootNode.descendentCount < this.maxNodeCount/100 && this.rootNode.mostLikelyUnexploredDescendent != nil && this.IsCurrentlySearching() {
				// Wait for more depth to be explored
				go func() {
					time.Sleep(time.Duration(100) * time.Millisecond)
					this.nextMoveChannelReceiver <- nextMoveChannel
				}()
			} else {
				nextMoveMap := extensions.ValueMap{}
				for childMove, childNode := range this.rootNode.children {
					nextMoveMap[childMove] = childNode.value
				}

				nextMoveChannel <- &nextMoveMap
			}

		case unexploredNodeReceiver := <-this.unexploredNodeReceiverChannel:
			unexploredNode := this.rootNode.mostLikelyUnexploredDescendent
			if unexploredNode != nil && this.rootNode.descendentCount < this.maxNodeCount {
				if !unexploredNode.incrementReference() { // This will be decremenented once it's processed out of exploredNodeChannel
					continue
				}

				if unexploredNode.explorationStatus != Unexplored {
					unexploredNode.PrintLineage()
					this.rootNode.PrintLineage()
					log.Fatal(fmt.Sprintf("%p is not in Unexplored state! State: %d\n", unexploredNode, unexploredNode.explorationStatus))
				}

				unexploredNode.setWaitingForExploration()

				unexploredNodeReceiver <- unexploredNode
			} else {
				time.Sleep(time.Duration(1) * time.Millisecond)
				this.unexploredNodeReceiverChannel <- unexploredNodeReceiver
			}
		}

		if this.rootNode.game.IsGameOver() {
			break
		}
	}

	for _, worker := range exploreNodeWorkers {
		worker.terminate = true
	}
}

func NewExpectimax(game Game, heuristic ExpectimaxHeuristic, calculateChildLikelihood ExpectimaxChildLikelihoodFunc, maxNodeCount int) *Expectimax {
	initNodeMemoryPool()

	return &Expectimax{
		game,
		heuristic,
		calculateChildLikelihood,
		NewBaseNode(game),
		make(chan (chan<- interface{}), 10),
		make(chan (chan<- *extensions.ValueMap), 10),
		nil,
		nil,
		maxNodeCount,
	}
}
