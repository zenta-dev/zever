// Package memory provides an in-memory workflow.Workflow implementation with synchronous step execution.
//
// Runs live only in the process map: everything is lost on restart, and
// concurrent replicas cannot share work. Use adapters/workflow/db
// for durability (DB-backed runs with lease reclaim across restarts).
package memory
