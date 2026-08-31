// Package scheduler queues concurrent inference requests and decides when to
// dispatch them to a backend, taking the manager's memory budget into
// account before allowing a model to move toward READY/BUSY.
package scheduler
