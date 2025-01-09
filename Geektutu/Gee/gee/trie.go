package gee

import "strings"

type Node struct {
	pattern 	string		// Route to match, ex: /p/:lang.
	part		string		// A part of route, ex: :lang.
	children	[]*Node		// All children Node.
	isWild		bool		// True if part[0] is ':' or '*'.
}

func (n *Node) matchChild(part string) *Node {
	for _, child := range n.children {
		if child.part == part || child.isWild {
			return child
		}
	}
	return nil
}

func (n *Node) matchChildren(part string) []*Node {
	nodes := make([]*Node, 0)
	for _, child := range n.children {
		if child.part == part || child.isWild {
			nodes = append(nodes, child)
		}
	}
	return nodes
}

func (n* Node) insert(pattern string, parts []string, height int) {
	if height == len(parts) {
		n.pattern = pattern
		return;
	}
	part := parts[height]
	child := n.matchChild(part)
	if child == nil {
		child = &Node{
			part: part,
			isWild: part[0] == ':' || part[0] == '*',
		}
		n.children = append(n.children, child)
	}
	child.insert(pattern, parts, height + 1)
}

func (n* Node) search(parts []string, height int) *Node {
	if height == len(parts) || strings.HasPrefix(n.part, "*") {
		if n.pattern == "" {
			// No handler in this Trie Node.
			return nil
		}
		return n
	}
	part := parts[height]
	children := n.matchChildren(part)

	for _, child := range children {
		if res := child.search(parts, height + 1); res != nil {
			return res
		}
	}
	return nil
}

