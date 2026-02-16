package toolexecutor

import (
	"fmt"
	"strings"
)

// ToolCategory represents a category of tools
type ToolCategory string

const (
	CategoryRead    ToolCategory = "read"
	CategoryWrite   ToolCategory = "write"
	CategoryShell   ToolCategory = "shell"
	CategoryWeb     ToolCategory = "web"
	CategorySpec    ToolCategory = "spec"
	CategoryGeneral ToolCategory = "general"
)

// AllCategories returns all valid tool categories
func AllCategories() []ToolCategory {
	return []ToolCategory{
		CategoryRead,
		CategoryWrite,
		CategoryShell,
		CategoryWeb,
		CategorySpec,
		CategoryGeneral,
	}
}

// IsValidCategory checks if a category is valid
func IsValidCategory(category string) bool {
	cat := ToolCategory(strings.ToLower(category))
	for _, valid := range AllCategories() {
		if cat == valid {
			return true
		}
	}
	return false
}

// ToolRegistry manages tool definitions and their categories
type ToolRegistry struct {
	tools      map[string]*ToolMetadata
	categories map[string]ToolCategory
}

// ToolMetadata contains metadata about a tool including its category
type ToolMetadata struct {
	Name        string
	Description string
	Category    ToolCategory
}

// NewToolRegistry creates a new tool registry
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools:      make(map[string]*ToolMetadata),
		categories: make(map[string]ToolCategory),
	}
}

// Register registers a tool with its metadata
func (tr *ToolRegistry) Register(name string, description string, category ToolCategory) error {
	if name == "" {
		return fmt.Errorf("tool name is required")
	}

	if category == "" {
		category = CategoryGeneral
	}

	if !IsValidCategory(string(category)) {
		return fmt.Errorf("invalid category: %s", category)
	}

	tr.tools[name] = &ToolMetadata{
		Name:        name,
		Description: description,
		Category:    category,
	}
	tr.categories[name] = category

	return nil
}

// Get retrieves tool metadata by name
func (tr *ToolRegistry) Get(name string) (*ToolMetadata, error) {
	tool, ok := tr.tools[name]
	if !ok {
		return nil, fmt.Errorf("tool not found: %s", name)
	}
	return tool, nil
}

// GetCategory returns the category of a tool
func (tr *ToolRegistry) GetCategory(toolName string) (ToolCategory, error) {
	category, ok := tr.categories[toolName]
	if !ok {
		return CategoryGeneral, fmt.Errorf("tool not found: %s", toolName)
	}
	return category, nil
}

// List returns all registered tools
func (tr *ToolRegistry) List() []*ToolMetadata {
	tools := make([]*ToolMetadata, 0, len(tr.tools))
	for _, tool := range tr.tools {
		tools = append(tools, tool)
	}
	return tools
}

// FilterByCategory returns tools in a specific category
func (tr *ToolRegistry) FilterByCategory(category ToolCategory) []*ToolMetadata {
	filtered := []*ToolMetadata{}
	for _, tool := range tr.tools {
		if tool.Category == category {
			filtered = append(filtered, tool)
		}
	}
	return filtered
}

// FilterByCategories returns tools in any of the specified categories
func (tr *ToolRegistry) FilterByCategories(categories []ToolCategory) []*ToolMetadata {
	categorySet := make(map[ToolCategory]bool)
	for _, cat := range categories {
		categorySet[cat] = true
	}

	filtered := []*ToolMetadata{}
	for _, tool := range tr.tools {
		if categorySet[tool.Category] {
			filtered = append(filtered, tool)
		}
	}
	return filtered
}

// CategoryMatcher checks if a tool matches category-based policy rules
type CategoryMatcher struct {
	registry *ToolRegistry
}

// NewCategoryMatcher creates a new category matcher
func NewCategoryMatcher(registry *ToolRegistry) *CategoryMatcher {
	return &CategoryMatcher{
		registry: registry,
	}
}

// MatchesCategory checks if a tool belongs to a category
func (cm *CategoryMatcher) MatchesCategory(toolName string, category ToolCategory) bool {
	tool, err := cm.registry.Get(toolName)
	if err != nil {
		return false
	}
	return tool.Category == category
}

// MatchesAnyCategory checks if a tool belongs to any of the specified categories
func (cm *CategoryMatcher) MatchesAnyCategory(toolName string, categories []ToolCategory) bool {
	tool, err := cm.registry.Get(toolName)
	if err != nil {
		return false
	}

	for _, cat := range categories {
		if tool.Category == cat {
			return true
		}
	}
	return false
}

// ApplyCategoryPolicy applies category-based policy rules
// Returns true if the tool is allowed based on category rules
func (cm *CategoryMatcher) ApplyCategoryPolicy(toolName string, allowCategories []ToolCategory, denyCategories []ToolCategory) bool {
	tool, err := cm.registry.Get(toolName)
	if err != nil {
		// Unknown tools are denied by default
		return false
	}

	// Check deny categories first (deny overrides allow)
	for _, denyCat := range denyCategories {
		if tool.Category == denyCat {
			return false
		}
	}

	// If no allow categories specified, allow all (except denied)
	if len(allowCategories) == 0 {
		return true
	}

	// Check allow categories
	for _, allowCat := range allowCategories {
		if tool.Category == allowCat {
			return true
		}
	}

	// Not in allow list
	return false
}
