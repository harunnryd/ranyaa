package plugin

import (
	"os"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDependencyResolver_BuildDependencyGraph(t *testing.T) {
	logger := zerolog.New(os.Stdout).Level(zerolog.Disabled)
	resolver := NewDependencyResolver(logger)

	t.Run("builds graph with no dependencies", func(t *testing.T) {
		plugins := []DiscoveredPlugin{
			{ID: "plugin1"},
			{ID: "plugin2"},
		}
		manifests := map[string]*PluginManifest{
			"plugin1": {ID: "plugin1", Name: "Plugin 1", Version: "1.0.0", Main: "main"},
			"plugin2": {ID: "plugin2", Name: "Plugin 2", Version: "1.0.0", Main: "main"},
		}

		graph := resolver.BuildDependencyGraph(plugins, manifests)

		assert.Len(t, graph.Nodes, 2)
		assert.Len(t, graph.Edges["plugin1"], 0)
		assert.Len(t, graph.Edges["plugin2"], 0)
	})

	t.Run("builds graph with dependencies", func(t *testing.T) {
		plugins := []DiscoveredPlugin{
			{ID: "plugin1"},
			{ID: "plugin2"},
			{ID: "plugin3"},
		}
		manifests := map[string]*PluginManifest{
			"plugin1": {
				ID:      "plugin1",
				Name:    "Plugin 1",
				Version: "1.0.0",
				Main:    "main",
			},
			"plugin2": {
				ID:      "plugin2",
				Name:    "Plugin 2",
				Version: "1.0.0",
				Main:    "main",
				Dependencies: []PluginDependency{
					{PluginID: "plugin1"},
				},
			},
			"plugin3": {
				ID:      "plugin3",
				Name:    "Plugin 3",
				Version: "1.0.0",
				Main:    "main",
				Dependencies: []PluginDependency{
					{PluginID: "plugin1"},
					{PluginID: "plugin2"},
				},
			},
		}

		graph := resolver.BuildDependencyGraph(plugins, manifests)

		assert.Len(t, graph.Nodes, 3)
		assert.Len(t, graph.Edges["plugin1"], 0)
		assert.Len(t, graph.Edges["plugin2"], 1)
		assert.Contains(t, graph.Edges["plugin2"], "plugin1")
		assert.Len(t, graph.Edges["plugin3"], 2)
		assert.Contains(t, graph.Edges["plugin3"], "plugin1")
		assert.Contains(t, graph.Edges["plugin3"], "plugin2")
	})
}

func TestDependencyResolver_DetectCycles(t *testing.T) {
	logger := zerolog.New(os.Stdout).Level(zerolog.Disabled)
	resolver := NewDependencyResolver(logger)

	t.Run("detects no cycles in acyclic graph", func(t *testing.T) {
		graph := &DependencyGraph{
			Nodes: map[string]*PluginManifest{
				"a": {ID: "a"},
				"b": {ID: "b"},
				"c": {ID: "c"},
			},
			Edges: map[string][]string{
				"a": {},
				"b": {"a"},
				"c": {"a", "b"},
			},
		}

		cycles := resolver.DetectCycles(graph)
		assert.Empty(t, cycles)
	})

	t.Run("detects simple cycle", func(t *testing.T) {
		graph := &DependencyGraph{
			Nodes: map[string]*PluginManifest{
				"a": {ID: "a"},
				"b": {ID: "b"},
			},
			Edges: map[string][]string{
				"a": {"b"},
				"b": {"a"},
			},
		}

		cycles := resolver.DetectCycles(graph)
		assert.NotEmpty(t, cycles)
	})

	t.Run("detects self-dependency", func(t *testing.T) {
		graph := &DependencyGraph{
			Nodes: map[string]*PluginManifest{
				"a": {ID: "a"},
			},
			Edges: map[string][]string{
				"a": {"a"},
			},
		}

		cycles := resolver.DetectCycles(graph)
		assert.NotEmpty(t, cycles)
	})

	t.Run("detects cycle in larger graph", func(t *testing.T) {
		graph := &DependencyGraph{
			Nodes: map[string]*PluginManifest{
				"a": {ID: "a"},
				"b": {ID: "b"},
				"c": {ID: "c"},
				"d": {ID: "d"},
			},
			Edges: map[string][]string{
				"a": {"b"},
				"b": {"c"},
				"c": {"d"},
				"d": {"b"}, // Cycle: b -> c -> d -> b
			},
		}

		cycles := resolver.DetectCycles(graph)
		assert.NotEmpty(t, cycles)
	})
}

func TestDependencyResolver_ValidateDependencies(t *testing.T) {
	logger := zerolog.New(os.Stdout).Level(zerolog.Disabled)
	resolver := NewDependencyResolver(logger)

	t.Run("validates all dependencies exist", func(t *testing.T) {
		graph := &DependencyGraph{
			Nodes: map[string]*PluginManifest{
				"a": {
					ID:      "a",
					Version: "1.0.0",
					Dependencies: []PluginDependency{
						{PluginID: "b"},
					},
				},
				"b": {
					ID:      "b",
					Version: "1.0.0",
				},
			},
			Edges: map[string][]string{
				"a": {"b"},
				"b": {},
			},
		}

		errors := resolver.ValidateDependencies(graph)
		assert.Empty(t, errors)
	})

	t.Run("detects missing dependency", func(t *testing.T) {
		graph := &DependencyGraph{
			Nodes: map[string]*PluginManifest{
				"a": {
					ID:      "a",
					Version: "1.0.0",
					Dependencies: []PluginDependency{
						{PluginID: "missing"},
					},
				},
			},
			Edges: map[string][]string{
				"a": {"missing"},
			},
		}

		errors := resolver.ValidateDependencies(graph)
		assert.NotEmpty(t, errors)
		assert.Contains(t, errors["a"].Error(), "missing dependency")
	})

	t.Run("validates version constraints", func(t *testing.T) {
		testCases := []struct {
			name        string
			depVersion  string
			constraint  string
			shouldError bool
		}{
			{"exact match", "1.0.0", "1.0.0", false},
			{"caret compatible", "1.2.3", "^1.0.0", false},
			{"caret incompatible major", "2.0.0", "^1.0.0", true},
			{"tilde compatible", "1.0.5", "~1.0.0", false},
			{"tilde incompatible minor", "1.1.0", "~1.0.0", true},
			{"greater than", "2.0.0", ">=1.0.0", false},
			{"less than", "0.9.0", "<1.0.0", false},
			{"range", "1.5.0", ">=1.0.0 <2.0.0", false},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				graph := &DependencyGraph{
					Nodes: map[string]*PluginManifest{
						"a": {
							ID:      "a",
							Version: "1.0.0",
							Dependencies: []PluginDependency{
								{PluginID: "b", Version: tc.constraint},
							},
						},
						"b": {
							ID:      "b",
							Version: tc.depVersion,
						},
					},
					Edges: map[string][]string{
						"a": {"b"},
						"b": {},
					},
				}

				errors := resolver.ValidateDependencies(graph)
				if tc.shouldError {
					assert.NotEmpty(t, errors)
				} else {
					assert.Empty(t, errors)
				}
			})
		}
	})
}

func TestDependencyResolver_TopologicalSort(t *testing.T) {
	logger := zerolog.New(os.Stdout).Level(zerolog.Disabled)
	resolver := NewDependencyResolver(logger)

	t.Run("sorts simple chain", func(t *testing.T) {
		// A -> B -> C
		graph := &DependencyGraph{
			Nodes: map[string]*PluginManifest{
				"a": {ID: "a"},
				"b": {ID: "b"},
				"c": {ID: "c"},
			},
			Edges: map[string][]string{
				"a": {},
				"b": {"a"},
				"c": {"b"},
			},
		}

		sorted, err := resolver.TopologicalSort(graph)
		require.NoError(t, err)
		require.Len(t, sorted, 3)

		// A should come before B, B should come before C
		aIdx := indexOf(sorted, "a")
		bIdx := indexOf(sorted, "b")
		cIdx := indexOf(sorted, "c")

		assert.True(t, aIdx < bIdx, "A should come before B")
		assert.True(t, bIdx < cIdx, "B should come before C")
	})

	t.Run("sorts diamond dependency", func(t *testing.T) {
		// A -> B, C -> D
		graph := &DependencyGraph{
			Nodes: map[string]*PluginManifest{
				"a": {ID: "a"},
				"b": {ID: "b"},
				"c": {ID: "c"},
				"d": {ID: "d"},
			},
			Edges: map[string][]string{
				"a": {},
				"b": {"a"},
				"c": {"a"},
				"d": {"b", "c"},
			},
		}

		sorted, err := resolver.TopologicalSort(graph)
		require.NoError(t, err)
		require.Len(t, sorted, 4)

		// A should come before B and C
		// B and C should come before D
		aIdx := indexOf(sorted, "a")
		bIdx := indexOf(sorted, "b")
		cIdx := indexOf(sorted, "c")
		dIdx := indexOf(sorted, "d")

		assert.True(t, aIdx < bIdx, "A should come before B")
		assert.True(t, aIdx < cIdx, "A should come before C")
		assert.True(t, bIdx < dIdx, "B should come before D")
		assert.True(t, cIdx < dIdx, "C should come before D")
	})

	t.Run("rejects graph with cycles", func(t *testing.T) {
		graph := &DependencyGraph{
			Nodes: map[string]*PluginManifest{
				"a": {ID: "a"},
				"b": {ID: "b"},
			},
			Edges: map[string][]string{
				"a": {"b"},
				"b": {"a"},
			},
		}

		_, err := resolver.TopologicalSort(graph)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cycle")
	})

	t.Run("sorts independent plugins", func(t *testing.T) {
		graph := &DependencyGraph{
			Nodes: map[string]*PluginManifest{
				"a": {ID: "a"},
				"b": {ID: "b"},
				"c": {ID: "c"},
			},
			Edges: map[string][]string{
				"a": {},
				"b": {},
				"c": {},
			},
		}

		sorted, err := resolver.TopologicalSort(graph)
		require.NoError(t, err)
		assert.Len(t, sorted, 3)
	})
}

func TestDependencyResolver_GetDependents(t *testing.T) {
	logger := zerolog.New(os.Stdout).Level(zerolog.Disabled)
	resolver := NewDependencyResolver(logger)

	t.Run("finds dependents", func(t *testing.T) {
		graph := &DependencyGraph{
			Nodes: map[string]*PluginManifest{
				"a": {ID: "a"},
				"b": {ID: "b"},
				"c": {ID: "c"},
			},
			Edges: map[string][]string{
				"a": {},
				"b": {"a"},
				"c": {"a"},
			},
		}

		dependents := resolver.GetDependents(graph, "a")
		assert.Len(t, dependents, 2)
		assert.Contains(t, dependents, "b")
		assert.Contains(t, dependents, "c")
	})

	t.Run("returns empty for no dependents", func(t *testing.T) {
		graph := &DependencyGraph{
			Nodes: map[string]*PluginManifest{
				"a": {ID: "a"},
				"b": {ID: "b"},
			},
			Edges: map[string][]string{
				"a": {"b"},
				"b": {},
			},
		}

		dependents := resolver.GetDependents(graph, "a")
		assert.Empty(t, dependents)
	})
}

// Helper function to find index of element in slice
func indexOf(slice []string, item string) int {
	for i, v := range slice {
		if v == item {
			return i
		}
	}
	return -1
}
