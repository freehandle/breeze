/*
Package social provides a generic validator node for social protocols
derived from the breeze network.

Social protocols are required to provide a state and state mutation
interface that is compatible with the socoial package. In particular
state mutations must implemente the Merger interface

	type Merger[T any] interface {
		Merge(...T) T
	}

that combines an arbitrary number of state mutations into a single
consolidated state mutation.

Besied the social protocol must implemente a Blocker valiador interface

	type Blocker[T Merger[T]] interface {
		Validate([]byte) bool
		Mutations() T
	}

that is capable of validation block actions and incorporating the
consequences of those actions in state transitions into a mutations
object compatible with the Merger interface.

Besides this the social protocol must provide a state object that
implements the Stateful interface

	type Stateful[T Merger[T], B Blocker[T]] interface {
		Validator(...T) B
		Incorporate(T)
		Shutdown()
		Checksum() crypto.Hash
		Clone() chan Stateful[T, B]
		Serialize() []byte
	}

The Validator method returns a Blocker compatible validator object.
Inrporate persists state mutations into the state. Checksum provides
a standartized checksum of the state object. Clone returns a channel
that will receive a clone of the state object. Serialize returns a
serialized representation of the state object.

This serialization can be used to recreate a identical copy of the
state by a function

	type StateFromBytes[T Merger[T], B Blocker[T]] func([]byte) (Stateful[T, B], bool)

# Launching Node

# The node is launched by calling the Launch function

func LaunchNode[M Merger[M], B Blocker[M]](ctx context.Context, cfg Configuration, newState StateFromBytes[M, B]) chan error {

chan error only receives information if the server is shutdown either by the
context cancelation or by an irretrivable error in the node.

LaunchNode will log using the stardard slog package. The client calling the
LaunchNode is responsible for the slog Default configuration.

# Node Configuation

The configuration of the node is provided by the Configuration struct

	type Configuration struct {
		// Hostname should be empty or local host for internet connections
		// or any other string for testing.
		Hostname string
		// Privatekey for the node. Used to sign connections and blocks.
		Credentials crypto.PrivateKey
		// AdminPort is the port for the admin interface.
		AdminPort int
		// Firewall is used to filter incoming connections.
		Firewall *socket.AcceptValidConnections
		// KeepNBlocks is the number of recent blocks to keep in memory. Should be
		// at least the number of blocks in a checksum window for the root breeze
		// protocol.
		KeepNBlocks int
		// ParentProtocolCode is the protocol code that is prvoiding validated blocks
		// for the current protocol.
		ParentProtocolCode uint32
		// NodeProtocolCode is the protocol code for the current social protocol.
		NodeProtocolCode uint32
		// RootBlockInterval is the time between root breeze network blocks.
		RootBlockInterval time.Duration
		// RootChecksumWindow is the number of blocks within a checksum windows in
		// the root breeze network.
		RootChecksumWindow int
		// CalculateCheckSum is true if the node should calculate the checksum for
		// the protocol state at the middle of the checksum window.
		CalculateCheckSum bool
		// BlocksSourcePort is the port to connect to to receive blocks from a
		// parent validator.
		BlocksSourcePort int
		// BlocksTargetPort is the port to listen on to provide blocks to child
		// validators.
		BlocksTargetPort int
		// TrustedProviders is a list of trusted providers for the current protocol.
		TrustedProviders []socket.TokenAddr
		// ProvidersSize is the number of providers to connect to receive new blocks.
		ProvidersSize int
		// MaxCheckpointLag is the maximum number of blocks difference between a
		// proposed block epoch and a checkpoint epoch as defined by the breeze
		// root netwrok
		MaxCheckpointLag uint64
	}
*/
package social
