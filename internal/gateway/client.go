package gateway

// Client handles communication with the Terminus gateway
// for policy evaluation of mutating git commands.
//
// This is a placeholder for the governed mode (Fold 2).
// When connected to a gateway, mutating commands are sent to
// POST /git/evaluate for policy decision before execution.
//
// Without a gateway connection, gitfence operates in pure
// read-only mode (all mutating commands blocked).
