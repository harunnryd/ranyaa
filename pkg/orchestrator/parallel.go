package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// SpawnerInterface defines the interface for spawning agents
type SpawnerInterface interface {
	Spawn(ctx context.Context, req SpawnRequest) (AgentResult, error)
}

// ParallelExecutor handles parallel execution of multiple agents
type ParallelExecutor struct {
	spawner SpawnerInterface
	logger  Logger
}

// NewParallelExecutor creates a new ParallelExecutor instance
func NewParallelExecutor(spawner SpawnerInterface, logger Logger) *ParallelExecutor {
	return &ParallelExecutor{
		spawner: spawner,
		logger:  logger,
	}
}

// Execute executes multiple agents in parallel based on the request
func (p *ParallelExecutor) Execute(ctx context.Context, req ParallelRequest) ([]AgentResult, error) {
	if len(req.Requests) == 0 {
		return []AgentResult{}, fmt.Errorf("no requests provided")
	}

	// Validate join strategy
	if req.JoinStrategy != JoinAll && req.JoinStrategy != JoinFirst && req.JoinStrategy != JoinAny {
		return []AgentResult{}, fmt.Errorf("invalid join strategy: %s", req.JoinStrategy)
	}

	// Validate on-fail strategy
	if req.OnFail != OnFailAbort && req.OnFail != OnFailContinue {
		return []AgentResult{}, fmt.Errorf("invalid on-fail strategy: %s", req.OnFail)
	}

	if p.logger != nil {
		p.logger.Info("Starting parallel execution",
			"num_agents", len(req.Requests),
			"join_strategy", req.JoinStrategy,
			"on_fail", req.OnFail)
	}

	startTime := time.Now()

	// Execute based on join strategy
	var results []AgentResult
	var err error

	switch req.JoinStrategy {
	case JoinAll:
		results, err = p.executeJoinAll(ctx, req)
	case JoinFirst:
		results, err = p.executeJoinFirst(ctx, req)
	case JoinAny:
		results, err = p.executeJoinAny(ctx, req)
	}

	duration := time.Since(startTime)

	if p.logger != nil {
		p.logger.Info("Parallel execution completed",
			"duration_ms", duration.Milliseconds(),
			"num_results", len(results),
			"success", err == nil)
	}

	return results, err
}

// executeJoinAll waits for all agents to complete
func (p *ParallelExecutor) executeJoinAll(ctx context.Context, req ParallelRequest) ([]AgentResult, error) {
	results := make([]AgentResult, len(req.Requests))
	errors := make([]error, len(req.Requests))
	var wg sync.WaitGroup

	// Create a context that can be cancelled if we need to abort
	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Launch all agents
	for i, spawnReq := range req.Requests {
		wg.Add(1)
		go func(index int, request SpawnRequest) {
			defer wg.Done()

			result, err := p.spawner.Spawn(execCtx, request)
			results[index] = result
			errors[index] = err

			// If on-fail is abort and we got an error, cancel all other agents
			if err != nil && req.OnFail == OnFailAbort {
				if p.logger != nil {
					p.logger.Error("Agent failed, aborting all", err,
						"agent_id", request.AgentID,
						"index", index)
				}
				cancel()
			}
		}(i, spawnReq)
	}

	// Wait for all to complete
	wg.Wait()

	// Check for errors
	var firstError error
	failedCount := 0
	for i, err := range errors {
		if err != nil {
			failedCount++
			if firstError == nil {
				firstError = err
			}
			if p.logger != nil {
				p.logger.Error("Agent execution failed", err,
					"agent_id", req.Requests[i].AgentID,
					"index", i)
			}
		}
	}

	// If on-fail is abort and we have errors, return error
	if req.OnFail == OnFailAbort && firstError != nil {
		return results, fmt.Errorf("parallel execution aborted: %w", firstError)
	}

	// If on-fail is continue, return results even with errors
	if failedCount > 0 && p.logger != nil {
		p.logger.Info("Parallel execution completed with failures",
			"failed_count", failedCount,
			"total_count", len(req.Requests))
	}

	return results, nil
}

// executeJoinFirst waits for the first agent to complete successfully
func (p *ParallelExecutor) executeJoinFirst(ctx context.Context, req ParallelRequest) ([]AgentResult, error) {
	resultChan := make(chan AgentResult, len(req.Requests))
	errorChan := make(chan error, len(req.Requests))

	// Create a context that can be cancelled when first completes
	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Launch all agents
	for _, spawnReq := range req.Requests {
		go func(request SpawnRequest) {
			result, err := p.spawner.Spawn(execCtx, request)
			if err != nil {
				errorChan <- err
			} else {
				resultChan <- result
			}
		}(spawnReq)
	}

	// Wait for first successful result or all to fail
	completedCount := 0
	for completedCount < len(req.Requests) {
		select {
		case result := <-resultChan:
			// Got first successful result, cancel others
			if p.logger != nil {
				p.logger.Info("First agent completed successfully",
					"agent_id", result.AgentID,
					"instance_id", result.InstanceID)
			}
			cancel()
			return []AgentResult{result}, nil

		case err := <-errorChan:
			completedCount++
			if p.logger != nil {
				p.logger.Error("Agent failed in join-first", err)
			}
			// Continue waiting for others

		case <-ctx.Done():
			return []AgentResult{}, fmt.Errorf("context cancelled: %w", ctx.Err())
		}
	}

	// All agents failed
	return []AgentResult{}, fmt.Errorf("all agents failed in join-first strategy")
}

// executeJoinAny waits for any agent to complete (success or failure)
func (p *ParallelExecutor) executeJoinAny(ctx context.Context, req ParallelRequest) ([]AgentResult, error) {
	resultChan := make(chan AgentResult, len(req.Requests))

	// Create a context that can be cancelled when any completes
	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Launch all agents
	for _, spawnReq := range req.Requests {
		go func(request SpawnRequest) {
			result, err := p.spawner.Spawn(execCtx, request)
			if err != nil {
				// Even on error, send the result with error info
				result.Success = false
				result.Error = err.Error()
			}
			resultChan <- result
		}(spawnReq)
	}

	// Wait for any result (success or failure)
	select {
	case result := <-resultChan:
		// Got first result (success or failure), cancel others
		if p.logger != nil {
			p.logger.Info("First agent completed (any strategy)",
				"agent_id", result.AgentID,
				"instance_id", result.InstanceID,
				"success", result.Success)
		}
		cancel()
		return []AgentResult{result}, nil

	case <-ctx.Done():
		return []AgentResult{}, fmt.Errorf("context cancelled: %w", ctx.Err())
	}
}

// ExecuteAndWait is a convenience method that wraps Execute with a background context
func (p *ParallelExecutor) ExecuteAndWait(req ParallelRequest) ([]AgentResult, error) {
	return p.Execute(context.Background(), req)
}
