package swell

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/freehandle/breeze/consensus/chain"
	"github.com/freehandle/breeze/consensus/permission"
	"github.com/freehandle/breeze/consensus/relay"
	"github.com/freehandle/breeze/crypto"
	"github.com/freehandle/breeze/middleware/admin"
	"github.com/freehandle/breeze/socket"
)

var swellTestConfig = SwellNetworkConfiguration{
	NetworkHash:      crypto.Hash{},
	MaxPoolSize:      10,
	MaxCommitteeSize: 10,
	BlockInterval:    10 * time.Millisecond,
	ChecksumWindow:   10,
	Permission:       permission.Permissionless{},
}

func newTestNetwork(count int) {
	for n := 0; n < count; n++ {
		socket.TCPNetworkTest.AddNode(fmt.Sprintf("node%d", n), 1, 1*time.Millisecond, 1e9)
	}
}

func lauchTestRelay(pk crypto.PrivateKey, host string) (*relay.Node, error) {
	cfg := &relay.Config{
		Credentials:       pk,
		GatewayPort:       4101,
		BlockListenerPort: 4102,
		Firewall:          relay.NewFireWall(nil, nil, true, true),
		Hostname:          host,
	}
	return relay.Run(context.Background(), cfg)
}

func lauchAdminPort(pk crypto.PrivateKey, node string) (*admin.Administration, error) {
	acceptAll := &socket.AcceptValidConnections{}
	return admin.OpenAdminPort(context.Background(), node, pk, 4200, acceptAll, acceptAll)
}

func validatorConfigTest(count int) ([]ValidatorConfig, error) {
	cfg := make([]ValidatorConfig, count)
	for number := 0; number < count; number++ {
		host := fmt.Sprintf("node%d", number)
		token, pk := crypto.RandomAsymetricKey()
		relay, err := lauchTestRelay(pk, host)
		if err != nil {
			return nil, fmt.Errorf("could not create relay%d: %v", number, err)
		}
		adm, err := lauchAdminPort(pk, host)
		if err != nil {
			return nil, fmt.Errorf("could not create adn%d: %v", number, err)
		}

		cfg[number] = ValidatorConfig{
			Credentials:    pk,
			Hostname:       host,
			SwellConfig:    swellTestConfig,
			Relay:          relay,
			Admin:          adm,
			TrustedGateway: []socket.TokenAddr{{Token: token, Addr: host}},
		}
	}
	return cfg, nil
}

func compareBlockHeders(h1, h2 chain.BlockHeader) bool {
	if h1.NetworkHash != h2.NetworkHash {
		fmt.Println("NetworkHash", h1.NetworkHash, " h2: ", h2.NetworkHash)
		return false
	}
	if h1.Epoch != h2.Epoch {
		fmt.Println("Epoch", h1.Epoch, " h2: ", h2.Epoch)
		return false
	}
	if h1.CheckPoint != h2.CheckPoint {
		fmt.Println("CheckPoint", h1.CheckPoint, " h2: ", h2.CheckPoint)
		return false
	}
	if h1.CheckpointHash != h2.CheckpointHash {
		fmt.Println("CheckpointHash", h1.CheckpointHash, " h2: ", h2.CheckpointHash)
		return false
	}
	if h1.Proposer != h2.Proposer {
		fmt.Println("Proposer", h1.Proposer, " h2: ", h2.Proposer)
		return false
	}
	if h1.ProposedAt.Compare(h2.ProposedAt) != 0 {
		fmt.Println("ProposedAt", h1.ProposedAt, " h2: ", h2.ProposedAt)
		return false
	}
	return true
}

func compareSeal(s1, s2 chain.BlockSeal) bool {
	if s1.Hash != s2.Hash {
		fmt.Println("Hash", s1.Hash, " h2: ", s2.Hash)
		return false
	}
	if s1.FeesCollected != s2.FeesCollected {
		fmt.Println("FeesCollected", s1.FeesCollected, " h2: ", s2.FeesCollected)
		return false
	}
	if s1.SealSignature != s2.SealSignature {
		fmt.Println("SealSignature", s1.SealSignature, " h2: ", s2.SealSignature)
		return false
	}
	return true
}

func compareBlock(b1, b2 *chain.CommitBlock) bool {
	if !compareBlockHeders(b1.Header, b2.Header) {
		fmt.Println("Header")
		return false
	}
	if b1.Actions.Len() != b2.Actions.Len() {
		fmt.Println("Actions", b1.Actions.Len(), " h2: ", b2.Actions.Len())
		return false
	}
	if !compareSeal(b1.Seal, b2.Seal) {
		fmt.Println("Seal")
		return false
	}
	return true
}

func compareChains(blocks int, chains ...*chain.Blockchain) bool {
	if len(chains[0].RecentBlocks) < blocks {
		return false
	}
	for block := 0; block < blocks; block++ {
		for i := 1; i < len(chains); i++ {
			if len(chains[i].RecentBlocks) < blocks {
				return false
			}
			if !compareBlock(chains[0].RecentBlocks[block], chains[i].RecentBlocks[block]) {
				return false
			}
		}
	}
	return true
}

func TestGenesisNode(t *testing.T) {
	fmt.Println("Iniciando")
	ctx, cancel := context.WithCancel(context.Background())
	newTestNetwork(3)
	nodesCfg, err := validatorConfigTest(3)
	if err != nil {
		t.Fatal(err)
	}

	nodes := []*SwellNode{NewGenesisNode(ctx, nodesCfg[0].Credentials, nodesCfg[0])}

	genesisNodeTokenAddr := socket.TokenAddr{
		Token: nodesCfg[0].Credentials.PublicKey(),
		Addr:  "node0:4102",
	}
	time.Sleep(20 * time.Millisecond)
	synced := make(chan *SwellNode, 2)
	err = FullSyncValidatorNode(ctx, nodesCfg[1], genesisNodeTokenAddr, synced)
	fmt.Println("Sync1")
	if err != nil {
		t.Fatal(err, "node1")
	}
	time.Sleep(30 * time.Millisecond)
	err = FullSyncValidatorNode(ctx, nodesCfg[2], genesisNodeTokenAddr, synced)
	if err != nil {
		t.Fatal(err, "node2")
	}
	for {
		node := <-synced
		nodes = append(nodes, node)
		if len(nodes) == 3 {
			break
		}
	}
	time.Sleep(6 * time.Second)
	if !compareChains(6, nodes[0].blockchain, nodes[1].blockchain, nodes[2].blockchain) {
		t.Fatal("chains are not equal")
	}
	if len(nodes[0].validators.order) != 3 {
		t.Fatal("invalid validators")
	}
	cancel()
}
