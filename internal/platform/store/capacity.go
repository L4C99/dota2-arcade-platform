package store

import (
	"context"
	"errors"
	"fmt"
)

var ErrInvalidDesiredCapacity = errors.New("desired capacity exceeds the Controller-reported hard maximum")

type NodeCapacity struct {
	NodeID   string
	Hard     int
	Desired  int
	Occupied int
}

// Capacity counts all attempts except the two proven resource-terminal states.
// A stale/offline report never changes this count.
func (s *Store) Capacity(ctx context.Context, nodeID string) (NodeCapacity, error) {
	var c NodeCapacity
	err := s.Pool.QueryRow(ctx, `SELECT n.id,r.hard_max_instances,n.desired_max_instances,
		(SELECT count(*) FROM allocations a WHERE a.node_id=n.id
		 AND a.state NOT IN ('reclaimed','released_no_effect'))
		FROM nodes n JOIN node_reports r ON r.node_id=n.id WHERE n.id=$1`, nodeID).
		Scan(&c.NodeID, &c.Hard, &c.Desired, &c.Occupied)
	return c, err
}

// SetDesiredCapacity is an operator control. Heartbeat owns hard capacity.
// Heartbeat takes the node lock before updating its report, so the check and
// update cannot race a new local hard maximum.
func (s *Store) SetDesiredCapacity(ctx context.Context, nodeID string, desired int) error {
	if desired < 0 {
		return ErrInvalidDesiredCapacity
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var hard int
	err = tx.QueryRow(ctx, `SELECT r.hard_max_instances FROM nodes n
		JOIN node_reports r ON r.node_id=n.id WHERE n.id=$1 FOR UPDATE OF n`, nodeID).Scan(&hard)
	if err != nil {
		return err
	}
	if desired > hard {
		return fmt.Errorf("%w: desired=%d hard=%d", ErrInvalidDesiredCapacity, desired, hard)
	}
	if _, err := tx.Exec(ctx, `UPDATE nodes SET desired_max_instances=$2 WHERE id=$1`, nodeID, desired); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
