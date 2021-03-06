package fsm

import (
	"github.com/hashicorp/consul/agent/consul/autopilot"
	"github.com/hashicorp/consul/agent/consul/state"
	"github.com/hashicorp/consul/agent/structs"
	"github.com/hashicorp/go-msgpack/codec"
	"github.com/hashicorp/raft"
)

func init() {
	registerPersister(persistOSS)

	registerRestorer(structs.RegisterRequestType, restoreRegistration)
	registerRestorer(structs.KVSRequestType, restoreKV)
	registerRestorer(structs.TombstoneRequestType, restoreTombstone)
	registerRestorer(structs.SessionRequestType, restoreSession)
	registerRestorer(structs.ACLRequestType, restoreACL)
	registerRestorer(structs.ACLBootstrapRequestType, restoreACLBootstrap)
	registerRestorer(structs.CoordinateBatchUpdateType, restoreCoordinates)
	registerRestorer(structs.PreparedQueryRequestType, restorePreparedQuery)
	registerRestorer(structs.AutopilotRequestType, restoreAutopilot)
	registerRestorer(structs.IntentionRequestType, restoreIntention)
	registerRestorer(structs.ConnectCARequestType, restoreConnectCA)
	registerRestorer(structs.ConnectCAProviderStateType, restoreConnectCAProviderState)
	registerRestorer(structs.ConnectCAConfigType, restoreConnectCAConfig)
	registerRestorer(structs.IndexRequestType, restoreIndex)
	registerRestorer(structs.ACLTokenSetRequestType, restoreToken)
	registerRestorer(structs.ACLPolicySetRequestType, restorePolicy)
}

func persistOSS(s *snapshot, sink raft.SnapshotSink, encoder *codec.Encoder) error {
	if err := s.persistNodes(sink, encoder); err != nil {
		return err
	}
	if err := s.persistSessions(sink, encoder); err != nil {
		return err
	}
	if err := s.persistACLs(sink, encoder); err != nil {
		return err
	}
	if err := s.persistKVs(sink, encoder); err != nil {
		return err
	}
	if err := s.persistTombstones(sink, encoder); err != nil {
		return err
	}
	if err := s.persistPreparedQueries(sink, encoder); err != nil {
		return err
	}
	if err := s.persistAutopilot(sink, encoder); err != nil {
		return err
	}
	if err := s.persistIntentions(sink, encoder); err != nil {
		return err
	}
	if err := s.persistConnectCA(sink, encoder); err != nil {
		return err
	}
	if err := s.persistConnectCAProviderState(sink, encoder); err != nil {
		return err
	}
	if err := s.persistConnectCAConfig(sink, encoder); err != nil {
		return err
	}
	if err := s.persistIndex(sink, encoder); err != nil {
		return err
	}
	return nil
}

func (s *snapshot) persistNodes(sink raft.SnapshotSink,
	encoder *codec.Encoder) error {

	// Get all the nodes
	nodes, err := s.state.Nodes()
	if err != nil {
		return err
	}

	// Register each node
	for node := nodes.Next(); node != nil; node = nodes.Next() {
		n := node.(*structs.Node)
		req := structs.RegisterRequest{
			ID:              n.ID,
			Node:            n.Node,
			Datacenter:      n.Datacenter,
			Address:         n.Address,
			TaggedAddresses: n.TaggedAddresses,
			NodeMeta:        n.Meta,
		}

		// Register the node itself
		if _, err := sink.Write([]byte{byte(structs.RegisterRequestType)}); err != nil {
			return err
		}
		if err := encoder.Encode(&req); err != nil {
			return err
		}

		// Register each service this node has
		services, err := s.state.Services(n.Node)
		if err != nil {
			return err
		}
		for service := services.Next(); service != nil; service = services.Next() {
			if _, err := sink.Write([]byte{byte(structs.RegisterRequestType)}); err != nil {
				return err
			}
			req.Service = service.(*structs.ServiceNode).ToNodeService()
			if err := encoder.Encode(&req); err != nil {
				return err
			}
		}

		// Register each check this node has
		req.Service = nil
		checks, err := s.state.Checks(n.Node)
		if err != nil {
			return err
		}
		for check := checks.Next(); check != nil; check = checks.Next() {
			if _, err := sink.Write([]byte{byte(structs.RegisterRequestType)}); err != nil {
				return err
			}
			req.Check = check.(*structs.HealthCheck)
			if err := encoder.Encode(&req); err != nil {
				return err
			}
		}
	}

	// Save the coordinates separately since they are not part of the
	// register request interface. To avoid copying them out, we turn
	// them into batches with a single coordinate each.
	coords, err := s.state.Coordinates()
	if err != nil {
		return err
	}
	for coord := coords.Next(); coord != nil; coord = coords.Next() {
		if _, err := sink.Write([]byte{byte(structs.CoordinateBatchUpdateType)}); err != nil {
			return err
		}
		updates := structs.Coordinates{coord.(*structs.Coordinate)}
		if err := encoder.Encode(&updates); err != nil {
			return err
		}
	}
	return nil
}

func (s *snapshot) persistSessions(sink raft.SnapshotSink,
	encoder *codec.Encoder) error {
	sessions, err := s.state.Sessions()
	if err != nil {
		return err
	}

	for session := sessions.Next(); session != nil; session = sessions.Next() {
		if _, err := sink.Write([]byte{byte(structs.SessionRequestType)}); err != nil {
			return err
		}
		if err := encoder.Encode(session.(*structs.Session)); err != nil {
			return err
		}
	}
	return nil
}

func (s *snapshot) persistACLs(sink raft.SnapshotSink,
	encoder *codec.Encoder) error {
	tokens, err := s.state.ACLTokens()
	if err != nil {
		return err
	}

	for token := tokens.Next(); token != nil; token = tokens.Next() {
		if _, err := sink.Write([]byte{byte(structs.ACLTokenSetRequestType)}); err != nil {
			return err
		}
		if err := encoder.Encode(token.(*structs.ACLToken)); err != nil {
			return err
		}
	}

	policies, err := s.state.ACLPolicies()
	if err != nil {
		return err
	}

	for policy := policies.Next(); policy != nil; policy = policies.Next() {
		if _, err := sink.Write([]byte{byte(structs.ACLPolicySetRequestType)}); err != nil {
			return err
		}
		if err := encoder.Encode(policy.(*structs.ACLPolicy)); err != nil {
			return err
		}
	}

	return nil
}

func (s *snapshot) persistKVs(sink raft.SnapshotSink,
	encoder *codec.Encoder) error {
	entries, err := s.state.KVs()
	if err != nil {
		return err
	}

	for entry := entries.Next(); entry != nil; entry = entries.Next() {
		if _, err := sink.Write([]byte{byte(structs.KVSRequestType)}); err != nil {
			return err
		}
		if err := encoder.Encode(entry.(*structs.DirEntry)); err != nil {
			return err
		}
	}
	return nil
}

func (s *snapshot) persistTombstones(sink raft.SnapshotSink,
	encoder *codec.Encoder) error {
	stones, err := s.state.Tombstones()
	if err != nil {
		return err
	}

	for stone := stones.Next(); stone != nil; stone = stones.Next() {
		if _, err := sink.Write([]byte{byte(structs.TombstoneRequestType)}); err != nil {
			return err
		}

		// For historical reasons, these are serialized in the snapshots
		// as KV entries. We want to keep the snapshot format compatible
		// with pre-0.6 versions for now.
		s := stone.(*state.Tombstone)
		fake := &structs.DirEntry{
			Key: s.Key,
			RaftIndex: structs.RaftIndex{
				ModifyIndex: s.Index,
			},
		}
		if err := encoder.Encode(fake); err != nil {
			return err
		}
	}
	return nil
}

func (s *snapshot) persistPreparedQueries(sink raft.SnapshotSink,
	encoder *codec.Encoder) error {
	queries, err := s.state.PreparedQueries()
	if err != nil {
		return err
	}

	for _, query := range queries {
		if _, err := sink.Write([]byte{byte(structs.PreparedQueryRequestType)}); err != nil {
			return err
		}
		if err := encoder.Encode(query); err != nil {
			return err
		}
	}
	return nil
}

func (s *snapshot) persistAutopilot(sink raft.SnapshotSink,
	encoder *codec.Encoder) error {
	autopilot, err := s.state.Autopilot()
	if err != nil {
		return err
	}
	if autopilot == nil {
		return nil
	}

	if _, err := sink.Write([]byte{byte(structs.AutopilotRequestType)}); err != nil {
		return err
	}
	if err := encoder.Encode(autopilot); err != nil {
		return err
	}
	return nil
}

func (s *snapshot) persistConnectCA(sink raft.SnapshotSink,
	encoder *codec.Encoder) error {
	roots, err := s.state.CARoots()
	if err != nil {
		return err
	}

	for _, r := range roots {
		if _, err := sink.Write([]byte{byte(structs.ConnectCARequestType)}); err != nil {
			return err
		}
		if err := encoder.Encode(r); err != nil {
			return err
		}
	}

	return nil
}

func (s *snapshot) persistConnectCAConfig(sink raft.SnapshotSink,
	encoder *codec.Encoder) error {
	config, err := s.state.CAConfig()
	if err != nil {
		return err
	}

	if _, err := sink.Write([]byte{byte(structs.ConnectCAConfigType)}); err != nil {
		return err
	}
	if err := encoder.Encode(config); err != nil {
		return err
	}
	return nil
}

func (s *snapshot) persistConnectCAProviderState(sink raft.SnapshotSink,
	encoder *codec.Encoder) error {
	state, err := s.state.CAProviderState()
	if err != nil {
		return err
	}

	for _, r := range state {
		if _, err := sink.Write([]byte{byte(structs.ConnectCAProviderStateType)}); err != nil {
			return err
		}
		if err := encoder.Encode(r); err != nil {
			return err
		}
	}
	return nil
}

func (s *snapshot) persistIntentions(sink raft.SnapshotSink,
	encoder *codec.Encoder) error {
	ixns, err := s.state.Intentions()
	if err != nil {
		return err
	}

	for _, ixn := range ixns {
		if _, err := sink.Write([]byte{byte(structs.IntentionRequestType)}); err != nil {
			return err
		}
		if err := encoder.Encode(ixn); err != nil {
			return err
		}
	}
	return nil
}

func (s *snapshot) persistIndex(sink raft.SnapshotSink, encoder *codec.Encoder) error {
	// Get all the indexes
	iter, err := s.state.Indexes()
	if err != nil {
		return err
	}

	for raw := iter.Next(); raw != nil; raw = iter.Next() {
		// Prepare the request struct
		idx := raw.(*state.IndexEntry)

		// Write out a node registration
		sink.Write([]byte{byte(structs.IndexRequestType)})
		if err := encoder.Encode(idx); err != nil {
			return err
		}
	}
	return nil
}

func restoreRegistration(header *snapshotHeader, restore *state.Restore, decoder *codec.Decoder) error {
	var req structs.RegisterRequest
	if err := decoder.Decode(&req); err != nil {
		return err
	}
	if err := restore.Registration(header.LastIndex, &req); err != nil {
		return err
	}
	return nil
}

func restoreKV(header *snapshotHeader, restore *state.Restore, decoder *codec.Decoder) error {
	var req structs.DirEntry
	if err := decoder.Decode(&req); err != nil {
		return err
	}
	if err := restore.KVS(&req); err != nil {
		return err
	}
	return nil
}

func restoreTombstone(header *snapshotHeader, restore *state.Restore, decoder *codec.Decoder) error {
	var req structs.DirEntry
	if err := decoder.Decode(&req); err != nil {
		return err
	}

	// For historical reasons, these are serialized in the
	// snapshots as KV entries. We want to keep the snapshot
	// format compatible with pre-0.6 versions for now.
	stone := &state.Tombstone{
		Key:   req.Key,
		Index: req.ModifyIndex,
	}
	if err := restore.Tombstone(stone); err != nil {
		return err
	}
	return nil
}

func restoreSession(header *snapshotHeader, restore *state.Restore, decoder *codec.Decoder) error {
	var req structs.Session
	if err := decoder.Decode(&req); err != nil {
		return err
	}
	if err := restore.Session(&req); err != nil {
		return err
	}
	return nil
}

func restoreACL(header *snapshotHeader, restore *state.Restore, decoder *codec.Decoder) error {
	var req structs.ACL
	if err := decoder.Decode(&req); err != nil {
		return err
	}

	if err := restore.ACLToken(req.Convert()); err != nil {
		return err
	}
	return nil
}

// DEPRECATED (ACL-Legacy-Compat) - remove once v1 acl compat is removed
func restoreACLBootstrap(header *snapshotHeader, restore *state.Restore, decoder *codec.Decoder) error {
	var req structs.ACLBootstrap
	if err := decoder.Decode(&req); err != nil {
		return err
	}

	// With V2 ACLs whether bootstrapping has been performed is stored in the index table like nomad
	// so this "restores" into that index table.
	return restore.IndexRestore(&state.IndexEntry{Key: "acl-token-bootstrap", Value: req.ModifyIndex})
}

func restoreCoordinates(header *snapshotHeader, restore *state.Restore, decoder *codec.Decoder) error {
	var req structs.Coordinates
	if err := decoder.Decode(&req); err != nil {
		return err
	}
	if err := restore.Coordinates(header.LastIndex, req); err != nil {
		return err
	}
	return nil
}

func restorePreparedQuery(header *snapshotHeader, restore *state.Restore, decoder *codec.Decoder) error {
	var req structs.PreparedQuery
	if err := decoder.Decode(&req); err != nil {
		return err
	}
	if err := restore.PreparedQuery(&req); err != nil {
		return err
	}
	return nil
}

func restoreAutopilot(header *snapshotHeader, restore *state.Restore, decoder *codec.Decoder) error {
	var req autopilot.Config
	if err := decoder.Decode(&req); err != nil {
		return err
	}
	if err := restore.Autopilot(&req); err != nil {
		return err
	}
	return nil
}

func restoreIntention(header *snapshotHeader, restore *state.Restore, decoder *codec.Decoder) error {
	var req structs.Intention
	if err := decoder.Decode(&req); err != nil {
		return err
	}
	if err := restore.Intention(&req); err != nil {
		return err
	}
	return nil
}

func restoreConnectCA(header *snapshotHeader, restore *state.Restore, decoder *codec.Decoder) error {
	var req structs.CARoot
	if err := decoder.Decode(&req); err != nil {
		return err
	}
	if err := restore.CARoot(&req); err != nil {
		return err
	}
	return nil
}

func restoreConnectCAProviderState(header *snapshotHeader, restore *state.Restore, decoder *codec.Decoder) error {
	var req structs.CAConsulProviderState
	if err := decoder.Decode(&req); err != nil {
		return err
	}
	if err := restore.CAProviderState(&req); err != nil {
		return err
	}
	return nil
}

func restoreConnectCAConfig(header *snapshotHeader, restore *state.Restore, decoder *codec.Decoder) error {
	var req structs.CAConfiguration
	if err := decoder.Decode(&req); err != nil {
		return err
	}
	if err := restore.CAConfig(&req); err != nil {
		return err
	}
	return nil
}

func restoreIndex(header *snapshotHeader, restore *state.Restore, decoder *codec.Decoder) error {
	var req state.IndexEntry
	if err := decoder.Decode(&req); err != nil {
		return err
	}
	return restore.IndexRestore(&req)
}

func restoreToken(header *snapshotHeader, restore *state.Restore, decoder *codec.Decoder) error {
	var req structs.ACLToken
	if err := decoder.Decode(&req); err != nil {
		return err
	}
	return restore.ACLToken(&req)
}

func restorePolicy(header *snapshotHeader, restore *state.Restore, decoder *codec.Decoder) error {
	var req structs.ACLPolicy
	if err := decoder.Decode(&req); err != nil {
		return err
	}
	return restore.ACLPolicy(&req)
}
