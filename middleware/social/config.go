package social

import (
	"time"

	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/middleware/config"
	"github.com/freehandle/breeze/socket"
)

// Configaration for a social node. Parent nodes is the nodes providing validated
// bloccks to the current node. Child nodes are the nodes receiving validated
// blocks from the current node. Root node refers to the relevant parameter for
// the root breeze network sequencing the blocks.
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

func (c Configuration) Check() error {
	return nil
}

func LoadJSONConfig(file string) (*Configuration, error) {
	return config.LoadConfig[Configuration](file)
}
