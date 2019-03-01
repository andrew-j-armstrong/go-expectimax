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
	game                     Game // Current game state
	heuristic                ExpectimaxHeuristic
	calculateChildLikelihood ExpectimaxChildLikelihoodFunc
	rootNode                 *expectimaxNode
	bestMoveChannelReceiver  chan (chan<- interface{})
	nextMoveChannelReceiver  chan (chan<- *extensions.ValueMap)
	maxNodeCount             int
}

func (this *Expectimax) processExploredNode(parent *expectimaxNode) {
	//fmt.Printf("Processing %p from the explored channel.\n", parent)
	parent.recursiveCalculateChildLikelihood(this.calculateChildLikelihood)
	parent.updateMostLikelyUnexploredDescendent()
	parent.updateAverageDepth()
	if parent.parent != nil {
		parent.parent.addDescendents(len(parent.children))
	}
	parent.explorationStatus = Archived
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

	return this.rootNode.descendentCount < this.maxNodeCount
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

func (this *Expectimax) RunExpectimax() {
	this.rootNode = NewBaseNode(this.game)

	moveListener := make(chan interface{}, 100)
	this.game.RegisterMoveListenerGeneric(moveListener)

	unexploredNodeReceiverChannel := make(chan chan<- *expectimaxNode, 100)
	exploredNodeChannel := make(chan *expectimaxNode, 100)
	exploreNodeWorkers := make([]*exploreNodeWorker, 0, 10)

	for i := 0; i < 10; i++ {
		exploreNodeWorker := NewExploreNodeWorker(unexploredNodeReceiverChannel, exploredNodeChannel)
		exploreNodeWorkers = append(exploreNodeWorkers, exploreNodeWorker)
		go exploreNodeWorker.ExploreNodeThread(this.heuristic)
	}

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
				this.rootNode.Explore(this.heuristic)
				this.processExploredNode(this.rootNode)
			case WaitingForExploration, Exploring:
				for this.rootNode.explorationStatus != Archived {
					exploredNode := <-exploredNodeChannel
					this.processExploredNode(exploredNode)
				}
			}

			this.rootNode = this.rootNode.descendToChild(move)

			if this.rootNode.game.IsGameOver() {
				break
			}

		case exploredNode := <-exploredNodeChannel:
			this.processExploredNode(exploredNode)

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

			if this.rootNode.descendentCount < this.maxNodeCount/100 && this.rootNode.mostLikelyUnexploredDescendent != nil {
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

		case unexploredNodeReceiver := <-unexploredNodeReceiverChannel:

			if !this.IsCurrentlySearching() {
				time.Sleep(time.Duration(50) * time.Millisecond)
				unexploredNodeReceiverChannel <- unexploredNodeReceiver
			} else {
				unexploredNode := this.rootNode.mostLikelyUnexploredDescendent

				if unexploredNode != nil {
					if unexploredNode.explorationStatus != Unexplored {
						unexploredNode.PrintLineage()
						log.Fatal(fmt.Sprintf("%p is not in Unexplored state! State: %d\n", unexploredNode, unexploredNode.explorationStatus))
					}

					unexploredNode.setWaitingForExploration()
					unexploredNodeReceiver <- unexploredNode
				} else {
					time.Sleep(time.Duration(5) * time.Millisecond)
					unexploredNodeReceiverChannel <- unexploredNodeReceiver
				}
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
		maxNodeCount,
	}
}
