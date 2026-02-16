package plugin

// ManifestSchema is the JSON Schema for plugin manifest validation
const ManifestSchema = `{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": ["id", "name", "version", "main"],
  "properties": {
    "id": {
      "type": "string",
      "pattern": "^[a-z0-9-]+$",
      "description": "Unique plugin identifier"
    },
    "name": {
      "type": "string",
      "minLength": 1,
      "description": "Human-readable plugin name"
    },
    "version": {
      "type": "string",
      "pattern": "^\\d+\\.\\d+\\.\\d+$",
      "description": "Semver version"
    },
    "description": {
      "type": "string",
      "description": "Plugin description"
    },
    "author": {
      "type": "string",
      "description": "Plugin author"
    },
    "main": {
      "type": "string",
      "minLength": 1,
      "description": "Entry point file path"
    },
    "dependencies": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["pluginId"],
        "properties": {
          "pluginId": {
            "type": "string",
            "minLength": 1
          },
          "version": {
            "type": "string",
            "description": "Semver constraint (e.g., ^1.0.0)"
          }
        }
      }
    },
    "permissions": {
      "type": "array",
      "items": {
        "type": "string",
        "enum": [
          "filesystem:read",
          "filesystem:write",
          "network:http",
          "network:websocket",
          "process:spawn",
          "database:read",
          "database:write",
          "gateway:register"
        ]
      }
    },
    "config": {
      "type": "object",
      "description": "JSON Schema for plugin configuration"
    },
    "exports": {
      "type": "object",
      "properties": {
        "tools": {
          "type": "array",
          "items": { "type": "string" }
        },
        "hooks": {
          "type": "array",
          "items": { "type": "string" }
        },
        "channels": {
          "type": "array",
          "items": { "type": "string" }
        },
        "providers": {
          "type": "array",
          "items": { "type": "string" }
        },
        "gatewayMethods": {
          "type": "array",
          "items": { "type": "string" }
        }
      }
    }
  }
}`
