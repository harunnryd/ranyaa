package plugin

import (
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/rs/zerolog"
)

// DependencyResolver resolves plugin dependencies and determines load order
type DependencyResolver struct {
	logger zerolog.Logger
}

// NewDependencyResolver creates a new dependency resolver
func NewDependencyResolver(logger zerolog.Logger) *DependencyResolver {
	return &DependencyResolver{
		logger: logger.With().Str("component", "dependency-resolver").Logger(),
	}
}

// BuildDependencyGraph builds a dependency graph from discovered plugins
func (r *DependencyResolver) BuildDependencyGraph(plugins []DiscoveredPlugin, manifests map[string]*PluginManifest) *DependencyGraph {
	graph := &DependencyGraph{
		Nodes: make(map[string]*PluginManifest),
		Edges: make(map[string][]string),
	}

	// Add all plugins as nodes
	for _, plugin := range plugins {
		if manifest, ok := manifests[plugin.ID]; ok {
			graph.Nodes[plugin.ID] = manifest
			graph.Edges[plugin.ID] = []string{}
		}
	}

	// Add edges for dependencies
	for pluginID, manifest := range graph.Nodes {
		for _, dep := range manifest.Dependencies {
			graph.Edges[pluginID] = append(graph.Edges[pluginID], dep.PluginID)
		}
	}

	return graph
}

// DetectCycles detects cycles in the dependency graph using DFS
// Returns a list of cycles, where each cycle is a list of plugin IDs
func (r *DependencyResolver) DetectCycles(graph *DependencyGraph) [][]string {
	var cycles [][]string
	visited := make(map[string]bool)
	recStack := make(map[string]bool)
	path := []string{}

	var dfs func(string) bool
	dfs = func(pluginID string) bool {
		visited[pluginID] = true
		recStack[pluginID] = true
		path = append(path, pluginID)

		for _, depID := range graph.Edges[pluginID] {
			if !visited[depID] {
				if dfs(depID) {
					return true
				}
			} else if recStack[depID] {
				// Found a cycle
				cycleStart := -1
				for i, id := range path {
					if id == depID {
						cycleStart = i
						break
					}
				}
				if cycleStart >= 0 {
					cycle := make([]string, len(path)-cycleStart)
					copy(cycle, path[cycleStart:])
					cycles = append(cycles, cycle)
				}
				return true
			}
		}

		path = path[:len(path)-1]
		recStack[pluginID] = false
		return false
	}

	for pluginID := range graph.Nodes {
		if !visited[pluginID] {
			dfs(pluginID)
		}
	}

	if len(cycles) > 0 {
		r.logger.Warn().Int("count", len(cycles)).Msg("Detected dependency cycles")
	}

	return cycles
}

// ValidateDependencies validates that all dependencies exist and versions are compatible
func (r *DependencyResolver) ValidateDependencies(graph *DependencyGraph) map[string]error {
	errors := make(map[string]error)

	for pluginID, manifest := range graph.Nodes {
		for _, dep := range manifest.Dependencies {
			// Check if dependency exists
			depManifest, exists := graph.Nodes[dep.PluginID]
			if !exists {
				errors[pluginID] = fmt.Errorf("missing dependency: %s", dep.PluginID)
				r.logger.Error().
					Str("plugin", pluginID).
					Str("dependency", dep.PluginID).
					Msg("Missing dependency")
				continue
			}

			// Check version compatibility if constraint is specified
			if dep.Version != "" {
				if err := r.checkVersionCompatibility(dep.PluginID, depManifest.Version, dep.Version); err != nil {
					errors[pluginID] = fmt.Errorf("incompatible dependency version for %s: %w", dep.PluginID, err)
					r.logger.Error().
						Str("plugin", pluginID).
						Str("dependency", dep.PluginID).
						Str("required", dep.Version).
						Str("actual", depManifest.Version).
						Msg("Incompatible dependency version")
				}
			}
		}
	}

	return errors
}

// checkVersionCompatibility checks if a version satisfies a constraint
func (r *DependencyResolver) checkVersionCompatibility(pluginID, version, constraint string) error {
	v, err := semver.NewVersion(version)
	if err != nil {
		return fmt.Errorf("invalid version %s: %w", version, err)
	}

	c, err := semver.NewConstraint(constraint)
	if err != nil {
		return fmt.Errorf("invalid version constraint %s: %w", constraint, err)
	}

	if !c.Check(v) {
		return fmt.Errorf("version %s does not satisfy constraint %s", version, constraint)
	}

	return nil
}

// TopologicalSort performs a topological sort on the dependency graph
// Returns plugin IDs in load order (dependencies before dependents)
func (r *DependencyResolver) TopologicalSort(graph *DependencyGraph) ([]string, error) {
	// Check for cycles first
	cycles := r.DetectCycles(graph)
	if len(cycles) > 0 {
		return nil, fmt.Errorf("cannot sort graph with cycles: %v", cycles)
	}

	var sorted []string
	visited := make(map[string]bool)
	temp := make(map[string]bool)

	var visit func(string) error
	visit = func(pluginID string) error {
		if temp[pluginID] {
			return fmt.Errorf("cycle detected at %s", pluginID)
		}
		if visited[pluginID] {
			return nil
		}

		temp[pluginID] = true

		// Visit dependencies first
		for _, depID := range graph.Edges[pluginID] {
			if err := visit(depID); err != nil {
				return err
			}
		}

		temp[pluginID] = false
		visited[pluginID] = true
		sorted = append(sorted, pluginID)

		return nil
	}

	// Visit all nodes
	for pluginID := range graph.Nodes {
		if !visited[pluginID] {
			if err := visit(pluginID); err != nil {
				return nil, err
			}
		}
	}

	r.logger.Debug().
		Int("count", len(sorted)).
		Strs("order", sorted).
		Msg("Computed load order")

	return sorted, nil
}

// GetDependents returns all plugins that depend on the given plugin
func (r *DependencyResolver) GetDependents(graph *DependencyGraph, pluginID string) []string {
	var dependents []string

	for id, deps := range graph.Edges {
		for _, depID := range deps {
			if depID == pluginID {
				dependents = append(dependents, id)
				break
			}
		}
	}

	return dependents
}
