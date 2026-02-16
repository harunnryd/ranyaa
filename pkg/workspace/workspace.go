// Package workspace provides file-based configuration and personality management
// for AI agents using Markdown and YAML files. It enables declarative agent
// behavior definition without code changes through hot-reload capabilities.
//
// The workspace manager monitors workspace files, loads and parses their content,
// validates structure, detects changes, and broadcasts events to other system components.
//
// Example usage:
//
//	config := workspace.WorkspaceConfig{
//		WorkspacePath: "~/.ranya/workspace",
//		OnReload: func(file *workspace.WorkspaceFile) {
//			log.Info().Str("file", file.RelativePath).Msg("File reloaded")
//		},
//	}
//
//	manager, err := workspace.NewWorkspaceManager(config)
//	if err != nil {
//		log.Fatal().Err(err).Msg("Failed to create workspace manager")
//	}
//
//	if err := manager.Init(); err != nil {
//		log.Fatal().Err(err).Msg("Failed to initialize workspace manager")
//	}
//	defer manager.Close()
//
//	// Listen for file changes
//	manager.On(workspace.EventFileChanged, func(payload interface{}) {
//		event := payload.(workspace.FileChangedPayload)
//		log.Info().Str("file", event.RelativePath).Msg("File changed")
//	})
//
//	// Get file content
//	content, ok := manager.GetFileContent("AGENTS.md")
//	if ok {
//		fmt.Println(content)
//	}
package workspace
