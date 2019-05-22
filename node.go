package expectimax

import (
	"fmt"
	"log"
	"math"
	"sync"

	"github.com/andrew-j-armstrong/go-extensions"
)

type explorationStatus int

const (
	Unexplored explorationStatus = iota
	WaitingForExploration
	Exploring
	Explored
	Archived
)

type expectimaxNode struct {
	game                                     Game
	parent                                   *expectimaxNode
	children                                 map[interface{}]*expectimaxNode
	childLikelihood                          extensions.ValueMap
	childExploreProbability                  extensions.ValueMap
	explorationStatus                        explorationStatus
	lastMove                                 interface{}
	heuristic                                float64
	value                                    float64
	mostLikelyUnexploredDescendent           *expectimaxNode
	mostLikelyUnexploredDescendentLikelihood float64
	descendentCount                          int
	averageDepth                             float64
	referenceCount                           int
	markedForDeletion                        bool
}

var expectimaxNodeMemoryPool *sync.Pool

func initNodeMemoryPool() {
	if expectimaxNodeMemoryPool == nil {
		expectimaxNodeMemoryPool = &sync.Pool{
			New: func() interface{} {
				node := new(expectimaxNode)
				node.reset()
				return node
			},
		}
	}
}

func getNewNode() *expectimaxNode {
	node := expectimaxNodeMemoryPool.Get().(*expectimaxNode)
	//node := &expectimaxNode{}
	//node.reset()
	return node
}

func (node *expectimaxNode) reset() {
	if node.mostLikelyUnexploredDescendent != nil && node.mostLikelyUnexploredDescendent != node {
		node.mostLikelyUnexploredDescendent.decrementReference()
	}

	node.game = nil
	node.parent = nil
	if node.children != nil {
		for move, child := range node.children {
			if child.parent == node {
				child.parent = nil
			}
			delete(node.children, move)
		}
	} else {
		node.children = make(map[interface{}]*expectimaxNode)
	}
	if node.childLikelihood != nil {
		node.childLikelihood.Clear()
	} else {
		node.childLikelihood = make(extensions.ValueMap)
	}
	if node.childExploreProbability != nil {
		node.childExploreProbability.Clear()
	} else {
		node.childExploreProbability = make(extensions.ValueMap)
	}
	node.explorationStatus = Unexplored
	node.lastMove = nil
	node.heuristic = 0.0
	node.value = 0.0
	node.mostLikelyUnexploredDescendent = node
	node.mostLikelyUnexploredDescendentLikelihood = 1.0
	node.descendentCount = 0
	node.averageDepth = 0
	node.referenceCount = 0
	node.markedForDeletion = false
}

func (node *expectimaxNode) incrementReference() bool {
	// Needs to be atomic
	if node.markedForDeletion {
		return false
	}
	node.referenceCount++
	return true
}

func (node *expectimaxNode) decrementReference() {
	node.referenceCount-- // Needs to be atomic
	if node.referenceCount == 0 && node.markedForDeletion {
		node.reset()
		expectimaxNodeMemoryPool.Put(node)
	}
}

func (node *expectimaxNode) deleteTree(exemptChildNode *expectimaxNode) {
	if !node.incrementReference() {
		return // Already marked for deletion
	}
	defer node.decrementReference()

	node.markedForDeletion = true
	for _, childNode := range node.children {
		childNode.parent = nil
		if childNode != exemptChildNode {
			childNode.deleteTree(nil)
		}
	}
}

func (node *expectimaxNode) descendToChild(move interface{}) *expectimaxNode {
	if !node.incrementReference() {
		log.Fatal("Trying to descend to a child after the parent has already been marked for deletion")
	}

	childNode := node.children[move]
	if !childNode.incrementReference() {
		log.Fatal("Trying to descend to a child after the parent has already been marked for deletion")
	}
	defer childNode.decrementReference()

	childNode.game = childNode.GetGame()
	childNode.parent = nil

	node.decrementReference()
	go node.deleteTree(childNode)

	return childNode
}

func (node *expectimaxNode) addDescendents(descendentCount int) {
	if !node.incrementReference() {
		return
	}
	defer node.decrementReference()

	node.descendentCount += descendentCount
	parent := node.parent
	if parent != nil {
		parent.addDescendents(descendentCount)
	}
}

func (node *expectimaxNode) Print() {
	node.print("", math.MaxInt32, 1.0)
}

func (node *expectimaxNode) PrintToDepth(depth int) {
	node.print("", depth, 1.0)
}

func (node *expectimaxNode) PrintLineage() {
	if !node.incrementReference() {
		return
	}
	defer node.decrementReference()

	node.print("", 0, 1.0)
	parent := node.parent
	if parent != nil {
		parent.PrintLineage()
	}
}

func (node *expectimaxNode) GetGame() Game {
	if !node.incrementReference() {
		return nil
	}
	defer node.decrementReference()

	if node.game != nil {
		return node.game.Clone().(Game)
	}

	parent := node.parent

	if parent == nil {
		return nil
	}

	game := parent.GetGame()
	if game == nil {
		return nil
	}

	game.MakeMove(node.lastMove)
	return game
}

func (node *expectimaxNode) print(key string, depth int, likelihood float64) {
	if !node.incrementReference() {
		return
	}
	defer node.decrementReference()

	if depth > 1 {
		depth--
		for childMove, childNode := range node.children {
			childNode.print(fmt.Sprintf("%s%d", key, childMove), depth, node.childLikelihood[childMove])
		}
	}
}

func (node *expectimaxNode) updateAverageDepth() {
	if !node.incrementReference() {
		return
	}
	defer node.decrementReference()

	if len(node.children) == 0 {
		node.averageDepth = 0
	} else {
		var averageDepth float64
		for _, childNode := range node.children {
			averageDepth += childNode.averageDepth
		}

		node.averageDepth = 1.0 + averageDepth/float64(len(node.children))
	}

	parent := node.parent
	if parent != nil {
		parent.updateAverageDepth()
	}
}

func (node *expectimaxNode) updateMostLikelyUnexploredDescendent(recursive bool, printDebug bool) {
	if !node.incrementReference() {
		return
	}
	defer node.decrementReference()

	var mostLikelyUnexploredDescendent *expectimaxNode
	var mostLikelyUnexploredDescendentLikelihood float64

	switch node.explorationStatus {
	case Unexplored:
		mostLikelyUnexploredDescendent = node
		mostLikelyUnexploredDescendentLikelihood = 1.0
	case WaitingForExploration, Exploring:
		mostLikelyUnexploredDescendent = nil
		mostLikelyUnexploredDescendentLikelihood = 0.0
	case Archived, Explored:
		mostLikelyUnexploredDescendent = nil
		mostLikelyUnexploredDescendentLikelihood = 0.0

		for childMove, child := range node.children {
			if child.mostLikelyUnexploredDescendent == nil || (child.explorationStatus != Unexplored && child.explorationStatus != Archived) {
				continue
			}

			exploreChildLikelihood := child.mostLikelyUnexploredDescendentLikelihood * node.childExploreProbability[childMove]

			if mostLikelyUnexploredDescendentLikelihood < exploreChildLikelihood {
				mostLikelyUnexploredDescendent = child.mostLikelyUnexploredDescendent
				mostLikelyUnexploredDescendentLikelihood = exploreChildLikelihood
			}
		}
	}

	if mostLikelyUnexploredDescendent != node.mostLikelyUnexploredDescendent || mostLikelyUnexploredDescendentLikelihood != node.mostLikelyUnexploredDescendentLikelihood {
		if node.mostLikelyUnexploredDescendent != nil && node.mostLikelyUnexploredDescendent != node {
			node.mostLikelyUnexploredDescendent.decrementReference()
		}
		if mostLikelyUnexploredDescendent != nil && mostLikelyUnexploredDescendent != node {
			mostLikelyUnexploredDescendent.incrementReference()
		}
		node.mostLikelyUnexploredDescendent = mostLikelyUnexploredDescendent
		node.mostLikelyUnexploredDescendentLikelihood = mostLikelyUnexploredDescendentLikelihood

		parent := node.parent

		if recursive && parent != nil {
			parent.updateMostLikelyUnexploredDescendent(true, printDebug)
		}
	}
}

func (node *expectimaxNode) setWaitingForExploration() {
	if !node.incrementReference() {
		return
	}
	defer node.decrementReference()

	node.explorationStatus = WaitingForExploration
	node.updateMostLikelyUnexploredDescendent(true, true)
}

func NewBaseNode(game Game) *expectimaxNode {
	node := getNewNode()
	node.game = game.Clone().(Game)
	return node
}

func (node *expectimaxNode) Explore(heuristic ExpectimaxHeuristic, calculateChildLikelihoodFunc ExpectimaxChildLikelihoodFunc) {
	if !node.incrementReference() {
		return
	}
	defer node.decrementReference()

	if node.mostLikelyUnexploredDescendent != nil && node.mostLikelyUnexploredDescendent != node {
		node.mostLikelyUnexploredDescendent.decrementReference()
	}

	node.explorationStatus = Exploring
	node.mostLikelyUnexploredDescendent = nil
	node.mostLikelyUnexploredDescendentLikelihood = 0.0
	nodeGame := node.GetGame()

	if nodeGame == nil {
		return
	}

	for _, move := range *nodeGame.GetPossibleMoves() {
		childGame := nodeGame.Clone().(Game)
		childGame.MakeMove(move)

		childHeuristic := heuristic(childGame)

		childNode := getNewNode()
		childNode.parent = node
		childNode.heuristic = childHeuristic
		childNode.value = childHeuristic
		childNode.lastMove = move

		node.children[move] = childNode
		node.childLikelihood[move] = 0
		node.childExploreProbability[move] = 0
	}

	node.descendentCount = len(node.children)
	node.averageDepth = 1.0
	node.explorationStatus = Explored

	node.calculateChildLikelihood(calculateChildLikelihoodFunc, false)
}

func (node *expectimaxNode) getChildValue(childMove interface{}) float64 {
	childNode, ok := node.children[childMove]
	if !ok {
		return 0.0
	}

	return childNode.value
}

func (node *expectimaxNode) calculateChildLikelihood(calculateChildLikelihoodFunc ExpectimaxChildLikelihoodFunc, recursive bool) {
	if !node.incrementReference() {
		return
	}
	defer node.decrementReference()

	calculateChildLikelihoodFunc(node.GetGame, node.getChildValue, &node.childLikelihood)

	for move, likelihood := range node.childLikelihood {
		node.childExploreProbability[move] = (0.1 / float64(len(node.childLikelihood))) + 0.9*likelihood // 10% spread for exploration regardless of likelihood
	}

	var value float64
	if len(node.children) == 0 {
		value = node.heuristic
	} else {
		for childMove, childNode := range node.children {
			value += node.childLikelihood[childMove] * childNode.value
		}
	}

	if math.IsNaN(value) {
		node.Print()
		log.Fatal("NaN value in recursiveCalculateChildLikelihood!")
	}

	parent := node.parent
	if recursive && value != node.value && parent != nil {
		node.value = value
		node.updateMostLikelyUnexploredDescendent(false, false)
		parent.calculateChildLikelihood(calculateChildLikelihoodFunc, true)
	} else {
		node.value = value
		node.updateMostLikelyUnexploredDescendent(recursive, false)
	}
}

func (node *expectimaxNode) processExploredNode(calculateChildLikelihoodFunc ExpectimaxChildLikelihoodFunc) {
	if !node.incrementReference() {
		return
	}
	defer node.decrementReference()

	node.explorationStatus = Archived

	parent := node.parent
	if parent != nil {
		if parent.incrementReference() {
			defer parent.decrementReference()
			parent.calculateChildLikelihood(calculateChildLikelihoodFunc, true)
			parent.updateAverageDepth()
			parent.addDescendents(len(node.children))
		}
	}
}
