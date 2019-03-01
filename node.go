package expectimax

import (
	"fmt"
	"log"
	"math"
	"sync"

	"github.com/carbon-12/go-extensions"
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
	node.game = nil
	node.parent = nil
	if node.children != nil {
		for key := range node.children {
			delete(node.children, key)
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
		fmt.Printf("%p: Deleted\n", node)
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
	childNode := node.children[move]
	childNode.game = childNode.GetGame()
	childNode.parent = nil
	node.deleteTree(childNode)
	return childNode
}

func (node *expectimaxNode) addDescendents(descendentCount int) {
	node.descendentCount += descendentCount
	if node.parent != nil {
		node.parent.addDescendents(descendentCount)
	}
}

func (node *expectimaxNode) Print() {
	node.print("", math.MaxInt32, 1.0)
}

func (node *expectimaxNode) PrintToDepth(depth int) {
	node.print("", depth, 1.0)
}

func (node *expectimaxNode) PrintLineage() {
	node.print("", 0, 1.0)
	if node.parent != nil {
		node.parent.PrintLineage()
	}
}

func (node *expectimaxNode) GetGame() Game {
	if !node.incrementReference() {
		return nil
	}
	defer node.decrementReference()

	if node.game != nil {
		return node.game.CloneGeneric().(Game)
	}

	if node.parent == nil {
		return nil
	}

	game := node.parent.GetGame()
	if game == nil {
		return nil
	}

	game.MakeMoveGeneric(node.lastMove)
	return game
}

func (node *expectimaxNode) print(key string, depth int, likelihood float64) {
	if !node.incrementReference() {
		return
	}
	defer node.decrementReference()

	fmt.Printf("%s: %p parent:%p likelihood:%f children:%d explorationStatus:%d heuristic:%f value:%f unexplored:%p likelihood:%f descendents:%d depth:%f\n", key, node, node.parent, likelihood, len(node.children), node.explorationStatus, node.heuristic, node.value, node.mostLikelyUnexploredDescendent, node.mostLikelyUnexploredDescendentLikelihood, node.descendentCount, node.averageDepth)
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

	if node.parent != nil {
		node.parent.updateAverageDepth()
	}
}

func (node *expectimaxNode) updateMostLikelyUnexploredDescendent() {
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
	case Explored, Archived:
		mostLikelyUnexploredDescendent = nil
		mostLikelyUnexploredDescendentLikelihood = 0.0

		for childMove, child := range node.children {
			if child.mostLikelyUnexploredDescendent == nil {
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
		node.mostLikelyUnexploredDescendent = mostLikelyUnexploredDescendent
		node.mostLikelyUnexploredDescendentLikelihood = mostLikelyUnexploredDescendentLikelihood

		if node.parent != nil {
			node.parent.updateMostLikelyUnexploredDescendent()
		}
	}
}

func (node *expectimaxNode) setWaitingForExploration() {
	node.explorationStatus = WaitingForExploration
	node.updateMostLikelyUnexploredDescendent()
}

func NewBaseNode(game Game) *expectimaxNode {
	node := getNewNode()
	node.game = game.CloneGeneric().(Game)
	return node
}

func (node *expectimaxNode) Explore(heuristic ExpectimaxHeuristic) {
	if !node.incrementReference() {
		return
	}
	defer node.decrementReference()

	fmt.Printf("%p: Exploring\n", node)

	node.explorationStatus = Exploring
	node.mostLikelyUnexploredDescendent = nil
	node.mostLikelyUnexploredDescendentLikelihood = 0.0
	nodeGame := node.GetGame()

	if nodeGame == nil {
		return
	}

	for _, move := range *nodeGame.GetPossibleMovesGeneric() {
		childGame := nodeGame.CloneGeneric().(Game)
		childGame.MakeMoveGeneric(move)

		childHeuristic := heuristic(childGame)

		childNode := getNewNode()
		childNode.parent = node
		childNode.heuristic = childHeuristic
		childNode.value = childHeuristic
		childNode.lastMove = move

		fmt.Printf("%p: Unexplored\n", childNode)

		node.children[move] = childNode
		node.childLikelihood[move] = 0
		node.childExploreProbability[move] = 0
	}

	node.descendentCount = len(node.children)
	node.averageDepth = 1.0
	node.explorationStatus = Explored

	fmt.Printf("%p: Explored\n", node)

	if node.explorationStatus == Explored && node.mostLikelyUnexploredDescendent == node {
		node.PrintLineage()
		log.Fatal("How did I get here???")
	}
}

func (node *expectimaxNode) getChildValue(childMove interface{}) float64 {
	childNode, ok := node.children[childMove]
	if !ok {
		return 0.0
	}

	return childNode.value
}

func (node *expectimaxNode) recursiveCalculateChildLikelihood(calculateChildLikelihood ExpectimaxChildLikelihoodFunc) {
	if !node.incrementReference() {
		return
	}
	defer node.decrementReference()

	calculateChildLikelihood(node.GetGame, node.getChildValue, &node.childLikelihood)

	for move, likelihood := range node.childLikelihood {
		node.childExploreProbability[move] = (0.1 / float64(len(node.childLikelihood))) + 0.9*likelihood
	}

	if len(node.children) == 0 {
		node.value = node.heuristic
	} else {
		node.value = 0.0
		for childMove, childNode := range node.children {
			node.value += node.childLikelihood[childMove] * childNode.value
		}
	}

	if math.IsNaN(node.value) {
		node.Print()
		log.Fatal("NaN value in recursiveCalculateChildLikelihood!")
	}

	if node.parent != nil {
		node.parent.recursiveCalculateChildLikelihood(calculateChildLikelihood)
	}
}
