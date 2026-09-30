package demo

import "fmt"

type Edge struct {
	Weight int
	Start  *Node
	End    *Node
}

type Node struct {
	Value float32
}

type Graph struct {
	Nodes []*Node
	Edges []Edge
}

func (g *Graph) String() string {
	return fmt.Sprintf("graph with %d nodes and %d edges", len(g.Nodes), len(g.Edges))
}
