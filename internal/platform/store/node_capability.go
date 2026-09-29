package store

import "context"

// These immutable labels are checked before any Node route can read or
// advance an execution. The HTTP caller's live lease is checked separately.
func (s *Store) RequiredCapabilityForJob(ctx context.Context, nodeID, jobID string) (string, error) {
	var capability string
	err := s.Pool.QueryRow(ctx, `SELECT required_capability FROM node_jobs WHERE node_id=$1 AND id=$2`, nodeID, jobID).Scan(&capability)
	return capability, err
}

func (s *Store) RequiredCapabilityForAllocation(ctx context.Context, nodeID, allocationID string) (string, error) {
	var capability string
	err := s.Pool.QueryRow(ctx, `SELECT j.required_capability FROM allocations a
		JOIN node_jobs j ON j.allocation_id=a.id AND j.kind='create'
		WHERE a.node_id=$1 AND a.id=$2`, nodeID, allocationID).Scan(&capability)
	return capability, err
}
