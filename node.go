package expectimax

import (
	"fmt"
	"log"
	"math"

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

func (node *expectimaxNode) GetGame() Game {
	if node.game != nil {
		return node.game.CloneGeneric().(Game)
	}

	if node.parent == nil {
		log.Fatal(fmt.Sprintf("can't find game for %p and have no parent", node))
	}

	game := node.parent.GetGame()
	game.MakeMoveGeneric(node.lastMove)
	return game
}

func (node *expectimaxNode) print(key string, depth int, likelihood float64) {
	fmt.Printf("%s: %p parent:%p likelihood:%f children:%d explorationStatus:%d heuristic:%f value:%f unexplored:%p likelihood:%f descendents:%d depth:%f\n", key, node, node.parent, likelihood, len(node.children), node.explorationStatus, node.heuristic, node.value, node.mostLikelyUnexploredDescendent, node.mostLikelyUnexploredDescendentLikelihood, node.descendentCount, node.averageDepth)
	if depth > 1 {
		depth--
		for childMove, childNode := range node.children {
			childNode.print(fmt.Sprintf("%s%d", key, childMove), depth, node.childLikelihood[childMove])
		}
	}
}

func (node *expectimaxNode) updateAverageDepth() {
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
	node := &expectimaxNode{
		game.CloneGeneric().(Game),
		nil,
		make(map[interface{}]*expectimaxNode),
		extensions.ValueMap{},
		extensions.ValueMap{},
		Unexplored,
		nil,
		0.0,
		0.0,
		nil,
		1.0,
		0,
		0.0,
	}

	node.mostLikelyUnexploredDescendent = node
	return node
}

func (node *expectimaxNode) Explore(heuristic ExpectimaxHeuristic) {
	node.explorationStatus = Exploring
	nodeGame := node.GetGame()

	for _, move := range *nodeGame.GetPossibleMovesGeneric() {
		childGame := nodeGame.CloneGeneric().(Game)
		childGame.MakeMoveGeneric(move)

		childHeuristic := heuristic(childGame)

		if math.IsNaN(childHeuristic) {
			log.Fatal("heuristic is NaN")
		}

		childNode := &expectimaxNode{
			nil,
			node,
			make(map[interface{}]*expectimaxNode),
			extensions.ValueMap{},
			extensions.ValueMap{},
			Unexplored,
			move,
			childHeuristic,
			childHeuristic,
			nil,
			1.0,
			0,
			0.0,
		}

		childNode.mostLikelyUnexploredDescendent = childNode

		node.children[move] = childNode
		node.childLikelihood[move] = 0
		node.childExploreProbability[move] = 0
	}

	node.descendentCount = len(node.children)
	node.averageDepth = 1.0
	node.explorationStatus = Explored
}

func (node *expectimaxNode) recursiveCalculateChildLikelihood(calculateChildLikelihood ExpectimaxChildLikelihoodFunc) {
	calculateChildLikelihood(node.GetGame, func(childMove interface{}) float64 { return node.children[childMove].value }, &node.childLikelihood)

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

func (node *expectimaxNode) updateValueFromChildren() {
	var minChildValue, maxChildValue, totalChildValue float64
	var haveMinMax bool
	for _, childNode := range node.children {
		if !haveMinMax || childNode.value < minChildValue {
			minChildValue = childNode.value
		}
		if !haveMinMax || childNode.value > maxChildValue {
			maxChildValue = childNode.value
		}

		haveMinMax = true

		totalChildValue += childNode.value
	}

	const minSpread float64 = 0.2 // Tweak this to adjust the balance of least to most likely node to explore

	totalChildValue -= float64(len(node.children)) * minChildValue

}
