package toolexecutor

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// CLIApprovalHandler handles approval requests via CLI prompts
type CLIApprovalHandler struct {
	reader io.Reader
	writer io.Writer
}

// NewCLIApprovalHandler creates a new CLI approval handler
func NewCLIApprovalHandler(reader io.Reader, writer io.Writer) *CLIApprovalHandler {
	return &CLIApprovalHandler{
		reader: reader,
		writer: writer,
	}
}

// RequestApproval prompts the user for approval via CLI
func (c *CLIApprovalHandler) RequestApproval(ctx context.Context, req ApprovalRequest) (ApprovalResponse, error) {
	// Display approval request
	c.displayApprovalRequest(req)

	// Create response channel
	responseChan := make(chan ApprovalResponse, 1)
	errorChan := make(chan error, 1)

	// Read user input in goroutine
	go func() {
		response, err := c.readUserInput(req)
		if err != nil {
			errorChan <- err
		} else {
			responseChan <- response
		}
	}()

	// Wait for response or timeout
	select {
	case response := <-responseChan:
		return response, nil

	case err := <-errorChan:
		return ApprovalResponse{}, err

	case <-ctx.Done():
		c.displayTimeout()
		return ApprovalResponse{
			Approved: false,
			Reason:   "timeout",
		}, ctx.Err()
	}
}

// displayApprovalRequest displays the approval request to the user
func (c *CLIApprovalHandler) displayApprovalRequest(req ApprovalRequest) {
	fmt.Fprintln(c.writer, "")
	fmt.Fprintln(c.writer, "╔════════════════════════════════════════════════════════════════╗")
	fmt.Fprintln(c.writer, "║              🔐 EXEC APPROVAL REQUIRED                        ║")
	fmt.Fprintln(c.writer, "╚════════════════════════════════════════════════════════════════╝")
	fmt.Fprintln(c.writer, "")
	fmt.Fprintf(c.writer, "  Command:    %s\n", req.Command)

	if len(req.Args) > 0 {
		fmt.Fprintf(c.writer, "  Args:       %v\n", req.Args)
	}

	if req.Cwd != "" {
		fmt.Fprintf(c.writer, "  Directory:  %s\n", req.Cwd)
	}

	if req.AgentID != "" {
		fmt.Fprintf(c.writer, "  Agent:      %s\n", req.AgentID)
	}

	if req.Timeout > 0 {
		fmt.Fprintf(c.writer, "  Timeout:    %v\n", req.Timeout)
	}

	if len(req.Context) > 0 {
		fmt.Fprintln(c.writer, "  Context:")
		for key, value := range req.Context {
			fmt.Fprintf(c.writer, "    %s: %s\n", key, value)
		}
	}

	fmt.Fprintln(c.writer, "")
	fmt.Fprint(c.writer, "  Approve this command? [y/N]: ")
}

// readUserInput reads and parses user input
func (c *CLIApprovalHandler) readUserInput(req ApprovalRequest) (ApprovalResponse, error) {
	scanner := bufio.NewScanner(c.reader)

	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return ApprovalResponse{}, fmt.Errorf("failed to read input: %w", err)
		}
		// EOF or no input
		return ApprovalResponse{
			Approved: false,
			Reason:   "no input provided",
		}, nil
	}

	input := strings.TrimSpace(strings.ToLower(scanner.Text()))

	var response ApprovalResponse
	switch input {
	case "y", "yes":
		response = ApprovalResponse{
			Approved: true,
			Reason:   "approved by user",
		}
		c.displayApproved()

		log.Info().
			Str("command", req.Command).
			Msg("Command approved via CLI")

	case "n", "no", "":
		response = ApprovalResponse{
			Approved: false,
			Reason:   "denied by user",
		}
		c.displayDenied()

		log.Info().
			Str("command", req.Command).
			Msg("Command denied via CLI")

	default:
		response = ApprovalResponse{
			Approved: false,
			Reason:   fmt.Sprintf("invalid input: %s", input),
		}
		c.displayInvalidInput(input)

		log.Warn().
			Str("command", req.Command).
			Str("input", input).
			Msg("Invalid input for approval")
	}

	return response, nil
}

// displayApproved displays approval confirmation
func (c *CLIApprovalHandler) displayApproved() {
	fmt.Fprintln(c.writer, "")
	fmt.Fprintln(c.writer, "  ✅ Command APPROVED")
	fmt.Fprintln(c.writer, "")
}

// displayDenied displays denial confirmation
func (c *CLIApprovalHandler) displayDenied() {
	fmt.Fprintln(c.writer, "")
	fmt.Fprintln(c.writer, "  ❌ Command DENIED")
	fmt.Fprintln(c.writer, "")
}

// displayInvalidInput displays invalid input message
func (c *CLIApprovalHandler) displayInvalidInput(input string) {
	fmt.Fprintln(c.writer, "")
	fmt.Fprintf(c.writer, "  ⚠️  Invalid input: %s (defaulting to DENY)\n", input)
	fmt.Fprintln(c.writer, "")
}

// displayTimeout displays timeout message
func (c *CLIApprovalHandler) displayTimeout() {
	fmt.Fprintln(c.writer, "")
	fmt.Fprintln(c.writer, "  ⏱️  Approval request TIMED OUT")
	fmt.Fprintln(c.writer, "")
}

// SetReader sets the input reader
func (c *CLIApprovalHandler) SetReader(reader io.Reader) {
	c.reader = reader
}

// SetWriter sets the output writer
func (c *CLIApprovalHandler) SetWriter(writer io.Writer) {
	c.writer = writer
}

// PromptWithTimeout prompts for approval with a visible countdown
func (c *CLIApprovalHandler) PromptWithTimeout(ctx context.Context, req ApprovalRequest, timeout time.Duration) (ApprovalResponse, error) {
	// Display approval request
	c.displayApprovalRequest(req)

	// Create response channel
	responseChan := make(chan ApprovalResponse, 1)
	errorChan := make(chan error, 1)

	// Read user input in goroutine
	go func() {
		response, err := c.readUserInput(req)
		if err != nil {
			errorChan <- err
		} else {
			responseChan <- response
		}
	}()

	// Create ticker for countdown
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	deadline := time.Now().Add(timeout)

	// Wait for response or timeout
	for {
		select {
		case response := <-responseChan:
			return response, nil

		case err := <-errorChan:
			return ApprovalResponse{}, err

		case <-ticker.C:
			remaining := time.Until(deadline)
			if remaining <= 0 {
				c.displayTimeout()
				return ApprovalResponse{
					Approved: false,
					Reason:   "timeout",
				}, context.DeadlineExceeded
			}

		case <-ctx.Done():
			c.displayTimeout()
			return ApprovalResponse{
				Approved: false,
				Reason:   "timeout",
			}, ctx.Err()
		}
	}
}
