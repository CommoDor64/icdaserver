.PHONY: all run-scripts run-main

run-scripts:
	@echo "Running scripts/main.go..."
	@go run scripts/main.go

run-main: run-scripts
	@echo "Running main.go..."
	@go run .

all: run-main
