package utils

import (
	"sort"

	"gopkg.in/yaml.v3"
)

func SortYAML(input []byte) ([]byte, error) {
	var node yaml.Node
	if err := yaml.Unmarshal(input, &node); err != nil {
		return nil, err
	}
	sortYAMLNode(&node)
	return yaml.Marshal(&node)
}

func sortYAMLNode(node *yaml.Node) {
	switch node.Kind {
	case yaml.MappingNode:
		if len(node.Content) == 0 {
			return
		}

		for i := 0; i < len(node.Content); i += 2 {
			sortYAMLNode(node.Content[i+1])
		}

		sortMappingPairs(node)

	case yaml.SequenceNode:
		for _, child := range node.Content {
			sortYAMLNode(child)
		}
	}
}

func sortMappingPairs(node *yaml.Node) {
	n := len(node.Content) / 2
	pairs := make([][2]*yaml.Node, n)
	for i := range pairs {
		pairs[i] = [2]*yaml.Node{node.Content[i*2], node.Content[i*2+1]}
	}

	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i][0].Value < pairs[j][0].Value
	})

	for i := range pairs {
		node.Content[i*2] = pairs[i][0]
		node.Content[i*2+1] = pairs[i][1]
	}
}
