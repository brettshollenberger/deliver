package main

import "fmt"

type Node struct {
	parent      *Node
	children    []*Node
	packageInfo *Package
}

func NewNode(packageInfo *Package) *Node {
	return &Node{
		packageInfo: packageInfo,
	}
}

func (this *Node) addChild(child *Node) {
	this.children = append(this.children, child)
	child.parent = this
}

func (this *Node) dumpIndent(indent int) {
	for i := 0; i < indent; i++ {
		fmt.Printf(" ")
	}
	this.packageInfo.dump()
	for _, child := range this.children {
		child.dumpIndent(indent + 2)
	}
}

func (this *Node) dump() {
	if this.packageInfo == nil {
		// If looking at all packages, there's a root placeholder node.
		for _, child := range this.children {
			child.dump()
		}
	} else {
		this.dumpIndent(0)
	}
}

type Conflicts struct {
	chosen     *Node
	changesets map[string][]*Node
}

func ResolveConflicts(root *Node) []*Package {
	resolved := []*Package{}
	queue := root.children[:]

	check := make(map[string]*Conflicts)

	// Process nodes in breadth-first order, so the first package in the
	// dependency tree has highest priority.
	for len(queue) > 0 {
		// Pop an item from the front of the queue.
		node := queue[0]
		queue = queue[1:len(queue)]

		pkg := node.packageInfo
		if conflicts, ok := check[pkg.Source]; ok {
			// We already saw this package, so add it to the list of packages that
			// requested this same changeset.
			same, _ := conflicts.changesets[pkg.getRef()]
			same = append(same, node)
			conflicts.changesets[pkg.getRef()] = same
		} else {
			// This is the first time we've seen this package, so take it as canonical.
			check[pkg.Source] = &Conflicts{
				chosen: node,
				changesets: map[string][]*Node{
					pkg.getRef(): []*Node{node},
				},
			}
		}

		queue = append(queue, node.children...)
	}

	// Warn the user about any conflicts.
	for source, conflicts := range check {
		if len(conflicts.changesets) == 1 {
			continue
		}

		resolved = append(resolved, conflicts.chosen.packageInfo)

		fmt.Printf("Warning: conflicting versions found for %s (* was chosen):\n", source)
		for _, nodeList := range conflicts.changesets {
			for _, node := range nodeList {
				prefix := "    "
				if node == conflicts.chosen {
					prefix = "(*) "
				}

				fmt.Printf("  %s%s\n", prefix, node.packageInfo.getRef())

				indent := "        "
				for parent := node.parent; parent != nil && parent.packageInfo != nil; parent = parent.parent {
					fmt.Printf("%s... from %s\n", indent, parent.packageInfo.Source)
					indent += "  "
				}
			}
		}
	}

	return resolved
}
