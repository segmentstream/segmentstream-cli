package cliresult

import (
	"encoding/json"
	"fmt"
	"io"
)

const (
	SchemaVersion = "2"

	ExitReady               = 0
	ExitGenericError        = 1
	ExitNeedsAuth           = 10
	ExitMisconfigured       = 11
	ExitMissingPrerequisite = 12
	ExitNeedsUserDecision   = 13
)

type Stage struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Current bool   `json:"current,omitempty"`
}

type Warning struct {
	ID          string `json:"id"`
	RequiredFor string `json:"required_for,omitempty"`
	Fix         string `json:"fix,omitempty"`
}

type Diagnostic struct {
	ID         string `json:"id"`
	Field      string `json:"field,omitempty"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

type Capabilities struct {
	AuthMethods []string `json:"auth_methods,omitempty"`
}

type NextActionInput struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Flag     string `json:"flag"`
	Label    string `json:"label"`
	Required bool   `json:"required"`
}

type NextActionAccept struct {
	Method  string            `json:"method"`
	Label   string            `json:"label"`
	Command string            `json:"command"`
	Value   string            `json:"value,omitempty"`
	Inputs  []NextActionInput `json:"inputs,omitempty"`
}

type NextAction struct {
	Type    string             `json:"type"`
	Stage   string             `json:"stage"`
	Command string             `json:"command,omitempty"`
	Reason  string             `json:"reason,omitempty"`
	Accepts []NextActionAccept `json:"accepts,omitempty"`
	Verify  string             `json:"verify,omitempty"`
}

type Envelope struct {
	SchemaVersion string       `json:"schema_version"`
	Ready         bool         `json:"ready"`
	Warehouse     *string      `json:"warehouse"`
	Capabilities  Capabilities `json:"capabilities"`
	Stages        []Stage      `json:"stages"`
	Warnings      []Warning    `json:"warnings,omitempty"`
	Diagnostics   []Diagnostic `json:"diagnostics,omitempty"`
	NextAction    NextAction   `json:"next_action"`
}

type ExitError struct {
	Code int
	Err  error
}

func (err ExitError) Error() string {
	if err.Err == nil {
		return ""
	}
	return err.Err.Error()
}

func WithExitCode(code int, err error) error {
	return ExitError{Code: code, Err: err}
}

func ExitCode(err error) int {
	if err == nil {
		return ExitReady
	}
	if coded, ok := err.(ExitError); ok {
		return coded.Code
	}
	return ExitGenericError
}

func WriteJSON(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("write json: %w", err)
	}
	return nil
}
