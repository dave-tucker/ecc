package consul

import (
	"fmt"
	"time"

	"github.com/socketplane/ecc/Godeps/_workspace/src/github.com/armon/go-metrics"
	"github.com/socketplane/ecc/Godeps/_workspace/src/github.com/hashicorp/consul/consul/structs"
)

// Session endpoint is used to manipulate sessions for KV
type Session struct {
	srv *Server
}

// Apply is used to apply a modifying request to the data store. This should
// only be used for operations that modify the data
func (s *Session) Apply(args *structs.SessionRequest, reply *string) error {
	if done, err := s.srv.forward("Session.Apply", args, args, reply); done {
		return err
	}
	defer metrics.MeasureSince([]string{"consul", "session", "apply"}, time.Now())

	// Verify the args
	if args.Session.ID == "" && args.Op == structs.SessionDestroy {
		return fmt.Errorf("Must provide ID")
	}
	if args.Session.Node == "" && args.Op == structs.SessionCreate {
		return fmt.Errorf("Must provide Node")
	}

	// If this is a create, we must generate the Session ID. This must
	// be done prior to appending to the raft log, because the ID is not
	// deterministic. Once the entry is in the log, the state update MUST
	// be deterministic or the followers will not converge.
	if args.Op == structs.SessionCreate {
		// Generate a new session ID, verify uniqueness
		state := s.srv.fsm.State()
		for {
			args.Session.ID = generateUUID()
			_, sess, err := state.SessionGet(args.Session.ID)
			if err != nil {
				s.srv.logger.Printf("[ERR] consul.session: Session lookup failed: %v", err)
				return err
			}
			if sess == nil {
				break
			}
		}
	}

	// Apply the update
	resp, err := s.srv.raftApply(structs.SessionRequestType, args)
	if err != nil {
		s.srv.logger.Printf("[ERR] consul.session: Apply failed: %v", err)
		return err
	}
	if respErr, ok := resp.(error); ok {
		return respErr
	}

	// Check if the return type is a string
	if respString, ok := resp.(string); ok {
		*reply = respString
	}
	return nil
}

// Get is used to retrieve a single session
func (s *Session) Get(args *structs.SessionSpecificRequest,
	reply *structs.IndexedSessions) error {
	if done, err := s.srv.forward("Session.Get", args, args, reply); done {
		return err
	}

	// Get the local state
	state := s.srv.fsm.State()
	return s.srv.blockingRPC(&args.QueryOptions,
		&reply.QueryMeta,
		state.QueryTables("SessionGet"),
		func() error {
			index, session, err := state.SessionGet(args.Session)
			reply.Index = index
			if session != nil {
				reply.Sessions = structs.Sessions{session}
			}
			return err
		})
}

// List is used to list all the active sessions
func (s *Session) List(args *structs.DCSpecificRequest,
	reply *structs.IndexedSessions) error {
	if done, err := s.srv.forward("Session.List", args, args, reply); done {
		return err
	}

	// Get the local state
	state := s.srv.fsm.State()
	return s.srv.blockingRPC(&args.QueryOptions,
		&reply.QueryMeta,
		state.QueryTables("SessionList"),
		func() error {
			var err error
			reply.Index, reply.Sessions, err = state.SessionList()
			return err
		})
}

// NodeSessions is used to get all the sessions for a particular node
func (s *Session) NodeSessions(args *structs.NodeSpecificRequest,
	reply *structs.IndexedSessions) error {
	if done, err := s.srv.forward("Session.NodeSessions", args, args, reply); done {
		return err
	}

	// Get the local state
	state := s.srv.fsm.State()
	return s.srv.blockingRPC(&args.QueryOptions,
		&reply.QueryMeta,
		state.QueryTables("NodeSessions"),
		func() error {
			var err error
			reply.Index, reply.Sessions, err = state.NodeSessions(args.Node)
			return err
		})
}
