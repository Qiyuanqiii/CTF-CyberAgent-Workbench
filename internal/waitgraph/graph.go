package waitgraph

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"
)

const (
	MaxNodeIDRunes = 256
	MaxActiveNodes = 4096
	MaxActiveEdges = 8192
)

var (
	ErrCycle            = errors.New("synchronous wait dependency would create a cycle")
	ErrReverseAgentWait = errors.New("lower runtime layer cannot synchronously wait on an Agent")
	ErrGraphCapacity    = errors.New("synchronous wait graph capacity exceeded")
	defaultProcessGraph = New()
)

type Kind string

const (
	KindAgent     Kind = "agent"
	KindTool      Kind = "tool"
	KindRetriever Kind = "retriever"
	KindStore     Kind = "store"
	KindRunner    Kind = "runner"
	KindModel     Kind = "model"
	KindExternal  Kind = "external"
)

func (k Kind) Valid() bool {
	switch k {
	case KindAgent, KindTool, KindRetriever, KindStore, KindRunner, KindModel, KindExternal:
		return true
	default:
		return false
	}
}

type Node struct {
	Kind Kind
	ID   string
}

func Agent(id string) Node     { return Node{Kind: KindAgent, ID: id} }
func Tool(id string) Node      { return Node{Kind: KindTool, ID: id} }
func Retriever(id string) Node { return Node{Kind: KindRetriever, ID: id} }
func Store(id string) Node     { return Node{Kind: KindStore, ID: id} }
func Runner(id string) Node    { return Node{Kind: KindRunner, ID: id} }
func Model(id string) Node     { return Node{Kind: KindModel, ID: id} }
func External(id string) Node  { return Node{Kind: KindExternal, ID: id} }

func (n Node) Validate() error {
	n.ID = strings.TrimSpace(n.ID)
	if !n.Kind.Valid() || n.ID == "" {
		return errors.New("wait node kind and id are required")
	}
	if !utf8.ValidString(n.ID) || strings.ContainsRune(n.ID, 0) || len([]rune(n.ID)) > MaxNodeIDRunes {
		return fmt.Errorf("wait node id must be valid UTF-8 without NUL and at most %d characters", MaxNodeIDRunes)
	}
	return nil
}

func (n Node) key() string {
	return string(n.Kind) + "\x00" + strings.TrimSpace(n.ID)
}

type Graph struct {
	mu        sync.Mutex
	edges     map[string]map[string]int
	nodes     map[string]int
	edgeCount int
}

func New() *Graph {
	return &Graph{edges: make(map[string]map[string]int), nodes: make(map[string]int)}
}

// Default returns the process-wide graph used by independently constructed
// control-plane services. Tests may inject an isolated graph through WithWaitGraph.
func Default() *Graph { return defaultProcessGraph }

// Acquire records a synchronous dependency until the returned release function
// is called. The graph never permits lower runtime layers to call back into an
// Agent synchronously, even when that edge would not yet form a cycle.
func (g *Graph) Acquire(from Node, to Node) (func(), error) {
	if g == nil {
		return nil, errors.New("synchronous wait graph is required")
	}
	from.ID = strings.TrimSpace(from.ID)
	to.ID = strings.TrimSpace(to.ID)
	if err := from.Validate(); err != nil {
		return nil, fmt.Errorf("invalid wait source: %w", err)
	}
	if err := to.Validate(); err != nil {
		return nil, fmt.Errorf("invalid wait target: %w", err)
	}
	if to.Kind == KindAgent && lowerRuntimeKind(from.Kind) {
		return nil, fmt.Errorf("%w: %s cannot wait on %s", ErrReverseAgentWait, from.Kind, to.Kind)
	}
	fromKey, toKey := from.key(), to.key()
	if fromKey == toKey {
		return nil, fmt.Errorf("%w: self dependency", ErrCycle)
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if g.pathExistsLocked(toKey, fromKey) {
		return nil, fmt.Errorf("%w: %s/%s -> %s/%s", ErrCycle, from.Kind, from.ID, to.Kind, to.ID)
	}
	neighbors := g.edges[fromKey]
	if neighbors == nil {
		neighbors = make(map[string]int)
		g.edges[fromKey] = neighbors
	}
	if neighbors[toKey] == 0 {
		newNodes := 0
		if g.nodes[fromKey] == 0 {
			newNodes++
		}
		if g.nodes[toKey] == 0 {
			newNodes++
		}
		if g.edgeCount+1 > MaxActiveEdges || len(g.nodes)+newNodes > MaxActiveNodes {
			if len(neighbors) == 0 {
				delete(g.edges, fromKey)
			}
			return nil, ErrGraphCapacity
		}
		g.edgeCount++
	}
	neighbors[toKey]++
	g.nodes[fromKey]++
	g.nodes[toKey]++

	var once sync.Once
	return func() {
		once.Do(func() { g.release(fromKey, toKey) })
	}, nil
}

func lowerRuntimeKind(kind Kind) bool {
	switch kind {
	case KindTool, KindRetriever, KindStore, KindRunner:
		return true
	default:
		return false
	}
}

func (g *Graph) pathExistsLocked(start string, target string) bool {
	seen := make(map[string]struct{}, len(g.nodes))
	stack := []string{start}
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current == target {
			return true
		}
		if _, ok := seen[current]; ok {
			continue
		}
		seen[current] = struct{}{}
		for next, references := range g.edges[current] {
			if references > 0 {
				stack = append(stack, next)
			}
		}
	}
	return false
}

func (g *Graph) release(from string, to string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	neighbors := g.edges[from]
	if neighbors == nil || neighbors[to] <= 0 {
		return
	}
	neighbors[to]--
	g.nodes[from]--
	g.nodes[to]--
	if neighbors[to] == 0 {
		delete(neighbors, to)
		g.edgeCount--
	}
	if len(neighbors) == 0 {
		delete(g.edges, from)
	}
	if g.nodes[from] == 0 {
		delete(g.nodes, from)
	}
	if g.nodes[to] == 0 {
		delete(g.nodes, to)
	}
}

type currentNodeKey struct{}

func WithCurrent(ctx context.Context, node Node) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, currentNodeKey{}, node)
}

func Current(ctx context.Context) (Node, bool) {
	if ctx == nil {
		return Node{}, false
	}
	node, ok := ctx.Value(currentNodeKey{}).(Node)
	if !ok || node.Validate() != nil {
		return Node{}, false
	}
	node.ID = strings.TrimSpace(node.ID)
	return node, true
}

func Enter(ctx context.Context, graph *Graph, fallbackFrom Node, target Node) (context.Context, func(), error) {
	if graph == nil {
		return ctx, nil, errors.New("synchronous wait graph is required")
	}
	from, ok := Current(ctx)
	if !ok {
		from = fallbackFrom
	}
	release, err := graph.Acquire(from, target)
	if err != nil {
		return ctx, nil, err
	}
	return WithCurrent(ctx, target), release, nil
}
