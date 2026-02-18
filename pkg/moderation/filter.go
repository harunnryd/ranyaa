package moderation

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/harun/ranya/internal/config"
)

// ContentFilter checks content against configured keywords and patterns.
type ContentFilter struct {
	enabled  bool
	keywords []string
	patterns []*regexp.Regexp
}

// New creates a new content filter.
func New(cfg config.ModerationConfig) (*ContentFilter, error) {
	patterns := make([]*regexp.Regexp, 0, len(cfg.BlockedPatterns))
	for _, p := range cfg.BlockedPatterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern %s: %w", p, err)
		}
		patterns = append(patterns, re)
	}

	return &ContentFilter{
		enabled:  cfg.Enabled,
		keywords: cfg.BlockedKeywords,
		patterns: patterns,
	}, nil
}

// CheckPrompt returns an error if the prompt contains blocked content.
func (f *ContentFilter) CheckPrompt(prompt string) error {
	if !f.enabled {
		return nil
	}

	normalized := strings.ToLower(prompt)
	for _, kw := range f.keywords {
		if strings.Contains(normalized, strings.ToLower(kw)) {
			return fmt.Errorf("prompt contains blocked keyword: %s", kw)
		}
	}
	for i, re := range f.patterns {
		if re.MatchString(prompt) {
			return fmt.Errorf("prompt matches blocked pattern #%d", i+1)
		}
	}
	return nil
}

// CheckResponse returns an error if the response contains blocked content.
func (f *ContentFilter) CheckResponse(response string) error {
	if !f.enabled {
		return nil
	}
	// For now, same logic as prompt check
	return f.CheckPrompt(response)
}
