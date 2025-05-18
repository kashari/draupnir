// Package radix provides a radix tree implementation for efficient string-based lookups.
package tree

// Node represents a node in the radix tree.
type Node struct {
	// The key segment stored at this node
	key string

	// The value stored at this node
	value any

	// Whether this node has a value
	hasValue bool

	// Child nodes indexed by their first byte
	children map[byte]*Node
}

// Tree represents a radix tree data structure.
type Tree struct {
	root *Node
	size int
}

// New creates a new radix tree.
func New() *Tree {
	return &Tree{
		root: &Node{
			children: make(map[byte]*Node),
		},
	}
}

// Insert adds a new key-value pair to the tree.
func (t *Tree) Insert(key string, value any) {
	if key == "" {
		return
	}

	t.insert(t.root, key, value)
	t.size++
}

// insert recursively adds a key-value pair to the tree.
func (t *Tree) insert(n *Node, key string, value any) {
	// Handle empty tree
	if len(n.children) == 0 {
		n.children[key[0]] = &Node{
			key:      key,
			value:    value,
			hasValue: true,
			children: make(map[byte]*Node),
		}
		return
	}

	// Find the matching child
	c, ok := n.children[key[0]]
	if !ok {
		// No matching child, create a new one
		n.children[key[0]] = &Node{
			key:      key,
			value:    value,
			hasValue: true,
			children: make(map[byte]*Node),
		}
		return
	}

	// Find common prefix length
	prefixLen := commonPrefixLen(key, c.key)

	// If the key is already in the tree
	if prefixLen == len(key) && prefixLen == len(c.key) {
		c.value = value
		c.hasValue = true
		return
	}

	// If the key is a prefix of the child key, split the child
	if prefixLen == len(key) {
		// Key is a prefix of child, so we need to split
		childSuffix := c.key[prefixLen:]

		// Update current child to have the new value
		oldValue := c.value
		oldHasValue := c.hasValue
		oldChildren := c.children

		c.key = key
		c.value = value
		c.hasValue = true

		// Create a new child with the remaining suffix
		c.children = make(map[byte]*Node)
		if childSuffix != "" {
			c.children[childSuffix[0]] = &Node{
				key:      childSuffix,
				value:    oldValue,
				hasValue: oldHasValue,
				children: oldChildren,
			}
		}
		return
	}

	// If the child key is a prefix of the key, recurse
	if prefixLen == len(c.key) {
		remainder := key[prefixLen:]
		t.insert(c, remainder, value)
		return
	}

	// Neither is a prefix of the other, so we need to split
	// Create a new node with the common prefix
	newNode := &Node{
		key:      key[:prefixLen],
		hasValue: false,
		children: make(map[byte]*Node),
	}

	// Add the existing child with its suffix
	childSuffix := c.key[prefixLen:]
	c.key = childSuffix
	newNode.children[childSuffix[0]] = c

	// Add the new key with its suffix
	keySuffix := key[prefixLen:]
	if keySuffix != "" {
		newNode.children[keySuffix[0]] = &Node{
			key:      keySuffix,
			value:    value,
			hasValue: true,
			children: make(map[byte]*Node),
		}
	} else {
		// The key ends at the split point
		newNode.value = value
		newNode.hasValue = true
	}

	// Replace the child in the parent
	n.children[key[0]] = newNode
}

// Get retrieves a value from the tree.
// Returns the value and a boolean indicating if the key was found.
func (t *Tree) Get(key string) (interface{}, bool) {
	node, found := t.findNode(key)
	if !found || !node.hasValue {
		return nil, false
	}
	return node.value, true
}

// findNode looks up a key in the tree.
func (t *Tree) findNode(key string) (*Node, bool) {
	if key == "" {
		return nil, false
	}

	n := t.root
	for {
		// If we're at a leaf node and haven't matched the key, it's not in the tree
		if len(n.children) == 0 {
			return nil, false
		}

		// Find the matching child
		c, ok := n.children[key[0]]
		if !ok {
			return nil, false
		}

		// If the child's key is longer than the remaining key, it's not in the tree
		if len(c.key) > len(key) {
			return nil, false
		}

		// Check if the child's key matches the start of the key
		if key[:len(c.key)] != c.key {
			return nil, false
		}

		// If we've matched the entire key, return the child
		if len(c.key) == len(key) {
			return c, true
		}

		// Move to the next part of the key
		key = key[len(c.key):]
		n = c
	}
}

// Delete removes a key from the tree.
func (t *Tree) Delete(key string) bool {
	deleted := t.delete(t.root, key)
	if deleted {
		t.size--
	}
	return deleted
}

// delete recursively removes a key from the tree.
func (t *Tree) delete(n *Node, key string) bool {
	if key == "" {
		return false
	}

	c, ok := n.children[key[0]]
	if !ok {
		return false
	}

	// If the key doesn't match the child, it's not in the tree
	if len(key) < len(c.key) || key[:len(c.key)] != c.key {
		return false
	}

	// If we've matched the whole key
	if len(key) == len(c.key) {
		// If this node has children, just mark it as not having a value
		if len(c.children) > 0 {
			if !c.hasValue {
				return false
			}
			c.hasValue = false
			c.value = nil
			return true
		}

		// Otherwise, remove the node from its parent
		delete(n.children, key[0])
		return true
	}

	// Recurse to the child
	remainder := key[len(c.key):]
	if t.delete(c, remainder) {
		// If the child now has no value and no children, remove it
		if !c.hasValue && len(c.children) == 0 {
			delete(n.children, key[0])
		} else if !c.hasValue && len(c.children) == 1 {
			for b, grandchild := range c.children {
				n.children[key[0]] = &Node{
					key:      c.key + grandchild.key,
					value:    grandchild.value,
					hasValue: grandchild.hasValue,
					children: grandchild.children,
				}
				delete(c.children, b)
			}
		}
		return true
	}
	return false
}

// Size returns the number of keys in the tree.
func (t *Tree) Size() int {
	return t.size
}

// Walk iterates over all key-value pairs in the tree.
func (t *Tree) Walk(fn func(key string, value interface{}) bool) {
	t.walk(t.root, "", fn)
}

// walk recursively traverses the tree.
func (t *Tree) walk(n *Node, prefix string, fn func(key string, value interface{}) bool) bool {
	if n.hasValue {
		fullKey := prefix + n.key
		if fn(fullKey, n.value) {
			return true
		}
	}

	for _, c := range n.children {
		if t.walk(c, prefix+n.key, fn) {
			return true
		}
	}
	return false
}

// LongestPrefix finds the longest prefix that matches the given key.
func (t *Tree) LongestPrefix(key string) (string, interface{}, bool) {
	if key == "" {
		return "", nil, false
	}

	var lastMatch *Node
	var lastMatchKey string

	n := t.root
	prefix := ""

	for {
		if n.hasValue {
			lastMatch = n
			lastMatchKey = prefix
		}

		// If we've consumed the entire key, exit
		if len(key) == 0 {
			break
		}

		// Find the matching child
		c, ok := n.children[key[0]]
		if !ok {
			break
		}

		// If the child's key is longer than the remaining key, no match
		if len(c.key) > len(key) {
			break
		}

		// Check if the child's key matches the start of the key
		if key[:len(c.key)] != c.key {
			break
		}

		// Update prefix and move down the tree
		prefix += c.key
		key = key[len(c.key):]
		n = c
	}

	if lastMatch == nil {
		return "", nil, false
	}
	return lastMatchKey, lastMatch.value, true
}

func commonPrefixLen(a, b string) int {
	i := 0
	max := min(len(b), len(a))
	for i < max && a[i] == b[i] {
		i++
	}
	return i
}
