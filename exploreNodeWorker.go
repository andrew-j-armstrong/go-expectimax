package expectimax

type exploreNodeWorker struct {
	unexploredNodeReceiverChannel chan<- (chan<- *expectimaxNode)
	exploredNodeChannel           chan<- *expectimaxNode
	terminate                     bool
}

func (worker *exploreNodeWorker) ExploreNodeThread(heuristic ExpectimaxHeuristic, calculateChildLikelihoodFunc ExpectimaxChildLikelihoodFunc) {
	unexploredNodeChannel := make(chan *expectimaxNode)
	for !worker.terminate {
		worker.unexploredNodeReceiverChannel <- unexploredNodeChannel
		parent := <-unexploredNodeChannel
		parent.Explore(heuristic, calculateChildLikelihoodFunc)
		worker.exploredNodeChannel <- parent
	}
}

func NewExploreNodeWorker(unexploredNodeReceiverChannel chan<- (chan<- *expectimaxNode), exploredNodeChannel chan<- *expectimaxNode) *exploreNodeWorker {
	return &exploreNodeWorker{unexploredNodeReceiverChannel, exploredNodeChannel, false}
}
