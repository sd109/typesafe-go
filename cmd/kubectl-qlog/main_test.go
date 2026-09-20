package main

import "testing"

func TestCommandCanBeConstructedWithoutExternalServices(t *testing.T) {
	command := newCommand()
	if command == nil {
		t.Fatal("newCommand returned nil")
	}
	if command.Use != "kubectl-qlog [resource...]" {
		t.Fatalf("Use = %q", command.Use)
	}
}
