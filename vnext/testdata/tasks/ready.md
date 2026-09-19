# Tasks

## T-0001
Objective: retain full tracker task metadata
State: ready
Dependencies: T-0000
Criteria:
- captures every required field
Checks:
- unit | go test ./internal/tracker
Writable paths:
- internal/tracker/**
Resources:
- browser:1
Evidence pointers:
- receipt:T-0001
