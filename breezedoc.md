# 1. Learn

## 1.1 Overview of Breeze

### 1.1.1 Introduction

Breeze is a crypto network designed to offer a dedicated Proof-of-Stake (PoS) consensus layer for other blockchains. In addition to the routine execution of actions that influence the economics of the fungible token governing PoS rules, there exists a singular (void) action within Breeze. This action, apart from settlement fees, does not impact the network's state. Its purpose is to enable users to submit actions related to more specialized protocols that operate on top of the Breeze consensus layer.

The Breeze network performs two primary functions: i) it sequences actions within a block and orders blocks among themselves, establishing a global consensus on the incorporation of actions up to a specific epoch; and ii) it assigns an approximate timestamp to actions by associating them with blocks known to be composed and published within a defined timeframe.

Specialized blockchains built atop Breeze extend these two fundamental functions to a broader and more sophisticated range of capabilities. Breeze itself does not validate actions on those specialized blockchains; nonetheless, they are not required to implement specific actions gathering and ordering consensus.

For example, Axé is a specialized blockchain that empowers users with a proof-of-authorship capacity. It allows cryptographic keys to be associated to an exclusive handle namespace, and offers the possibility to grant or revoke to keys the power to sign on behalf of its authority. Axé itself is a base layer for more specialized protocols since this capacity is meaningless if no further capacities are offered. Thus Axé actions, themselves void actions on breeze protocol have a general purpose axé void action that can be used by even more specialized protocols implementing a rich capacity for end users to interact with each others in the digital arena.

For example, Axé is a specialized blockchain that empowers users with a proof-of-authorship capacity. It allows cryptographic keys to be associated with an exclusive handle namespace and offers the possibility to grant or revoke, to keys, the power to sign on behalf of its authority. Axé itself serves as a base layer for more specialized protocols, as this capacity is meaningless without additional capabilities. Thus, Axé actions, which are void actions on the Breeze protocol, include a general-purpose Axé void action. This action can be employed by even more specialized protocols to implement a rich capacity for end users to interact with each other in the digital arena.

### 1.1.2 Social Protocols

The first bytes of a void action on Breeze declare the protocol code implemented by the action. Each specialized blockchain should only concern itself with the protocol codes that apply to it. For instance, if the first bit of the code bytes is zero, Axé will interpret this action as a valid Axé action and attempt to validate it.

We refer to all protocols built on top of the Breeze consensus layer as social protocols. The intention is to distinguish them from monetary protocols prevalent in networks offering smart contract functionality. Since Breeze void actions do not involve the transfer of Breeze fungible tokens, the protocols derived from it cannot directly alter the ownership of fungible tokens on the underlying network.

Although monetary functions are one of the core reasons for the existence of crypto networks, their decentralization properties can obviously have a much wider impact. Breeze is an attempt to dramatically reduce complexity in experimenting with protocols governing hash spaces like namespaces or NFTs, the right to perform actions, and the capacity to create specialized digital venues, among several other possibilities that might not involve direct monetary transactions.

Smart contracts also enable almost indiscriminate interactions among different functionalities: one contract might call another and so on. The only base functionality offered by the Breeze network for mutual interactions between social protocols is the process of protocol pipe. For example, Aieh is another social protocol that runs on top of the Axé protocol and implements the functionality of moderated and private stages for digital interactions. It itself has a void action that can be used by even more specialized protocols. For instance, a protocol for a forum can be run inside an Aieh void protocol within an Axé void protocol within a Breeze void protocol.

## 1.1.3 Standalone Social Protocol Validators

Social protocols must have validators enforcing their rules and keeping track of their underlying state. For instance, an Axé validator must maintain a record of valid powers of attorney to accept an attorney's signature as proof-of-authorship. It also must keep track of handles already claimed in the namespace.

For pure social protocols that have no interaction with the outside world, relying solely on the action stream provided by the Breeze network, there is no immediate need for an extra consensus layer for validation. Each honest node, receiving information from the Breeze network, will validate the same actions and transition to the same state as long as the consensus over the Breeze network is functioning.

In this case, it is natural to have standalone social protocol validators that, as far as their users have confidence in their output, are considered reliable sources of authority over the social protocol-derived blockchain. Block hashes and state checksums can be calculated and broadcasted among different validators to ensure the validity of their outputs.

This is a very desirable property when extreme scaling is required on complex protocols. Since nodes can concentrate their resources solely on the validation of a stream of actions provided by their parent blockchain, horizontal scaling can be achieved more easily.

The Breeze codebase has a ready-to-use standalone social protocol validator for any Golang codebase that implements its own social interface. Thus, it can be very easy to experiment with new protocols even at large scales.

### 1.1.4 Pools of Social Protocol Validators

Nonetheless, there can be use cases where an additional consensus layer on a social protocol level is desirable. This is the case, for example, when one needs to gather information from the internet, generate random numbers, or interact with other blockchains in a permissionless setting.

In those cases, it might be reasonable to leverage Breeze's Swell consensus algorithm implementation to deploy blockchains for social protocols. If they are to be based on a Proof-of-Stake permission scheme, the social protocol must implement its own governing fungible tokens. This can be done within the protocol itself or through a smart contract in another blockchain.

In either case, the Breeze codebase provides the desired functionality to deploy specialized Proof-of-Stake blockchains for social protocols with minimum effort, as long as the codebase implements not only the social protocol interface but also the fungible token interface and the permission interface. In the case where fungible tokens are deployed as smart contracts, wrappers for the methods over the smart contracts must be provided.

## 1.2 Aero

Aero is the name of the fungible tokens within breeze network. The economics of aero supply will be decided before the genesis of the genesis mainet launch. On the testnets one billion aeros are minted to a unique token that is the first validator of the breeze network. You can request as many token as desired by this link. 

### 1.2.1 Role of Aero

The primary purpose of Aero is not to be a general conduit of monetary value but rather to serve as a substrate intermediating the economics of the Breeze network infrastructure. The Breeze ecosystem was designed to be as modular as possible, and as a rule of thumb, anyone offering services that consume real economic resources in terms of energy, hardware, or communication should be remunerated in Aeros.

For example, if one requests a blockchain database to send information over a range of blocks, it is reasonable that such a request should be accompanied by a promise to transfer Aeros to the database provider. At the testnet level, we are offering several services free of charge to facilitate experimentation within the network. However, as the network matures, one might expect that not all relevant services will be offered on the infrastructure free of charge. Nonetheless, all efforts are deployed to keep these costs extremely low.

The most basic functionality for Aero is to pay for processing fees for actions submitted to the Breeze network. Gateways might offer the service to pay for those fees themselves since they are more in touch with prevalent costs over the network.

Aero can thus have a much higher velocity of circulation than most traditional cryptocurrencies that have vast portions of their tokens hoarded for long-term investment purposes.

### 1.2.2 Proof-of-Stake

Consensus on crytpo networks is the property that different honest nodes on the network agree with each other over the correct sequence of blocks that constitutes the block chains and over the content of each block. There might be periods over which consensus is broken but a resilient network should be able to achieve consensus within reasonable time frame. 

Reliability on consensys derives from manifold aspects: the specific rules and parameters of the consus algorithm, the economic costs to corrupt the alogorithm, the economic, political or social benefits derived from temporarily or permanently corrupting consensus, the reliability of connectivity among the parts involved in the consensus, the reliability of the code bases running the consensus engines,  and finally the safekeeping of those cryptographic secrets necessary to participate in consensus. 

On networks the leverages on the capacity of holding and transfering large ammounts of real economic value on their tokens, consensus reliability and security is a major requirement. On other networks, like breeze, where scalability and resilience is of uppermost concern, temporary incapacity to enforce consensus is not the end of the world as long as resolution mechanisms exists that under litigations there are automated means to rollback to consistent state and keep moving on. 

Breeze deploys a specialized consensus algorithm, Swell, that is a minnor modification of tendermint bft alogorithm. It is designed to provide a continues flow of blocks and defines rollback mechanisms for litigation over consensus. Due to its resiliency it can opperate under much more agressive parameters than tendermint, even on a stricly permissionless environment. 

As every Proof-of-Stake consensus, the reliability is direclty linked to the guarantee that ample majority of stake is on the hands of honest and concious players that runs validators nodes over reliable hardware, software and network conditions. 

### 1.3 Swell consensus protocol

Swell is an original consensus protocol that is a slight variation of tendermint. It is designed to remain resilient under much more agressive parameters. Consensus committees in swell should be small in order to reduce the network burden of timelly sharing a block among several nodes. 

### 1.3.1 Checksum windows

The main difference is that swell operates under the concept of checksum windows. Within a checksum window there is a fixed set of validators that communicates with each others according to protocol rules and are incumbent on producing a certain number of blocks at specific timestamps. 

So the n-th block on the checksum window is expected to be minted between ti + n×ΔT and ti + (n+1)×ΔT, where ti is the initial timestamp of the cheksum windows. 

If for some reason the committee is not capable of producing consensus over the blocks for checksum window than the network is instructed to revert to the state at the start of the checksum window and the committee responsible for the formation of the previous valid checksum windows will be incumbent on performing that task. 

### 1.3.2 Checkpoint

Since every block on a checksum window is timestamped, a node responsible for building a certain block must perform the task without necessarily having consensus formed on the previous block. Thus, every block on Swell is formed against a certain checkpoint, which represents a block for which the node proposing the new block is in possession of consensus evidence and which is incorporated into the blockchain (meaning that there is also evidence for consensus on all the previous blocks).

At the end of the timeframe for the block, a consensus committee is formed to agree on the hash of this newly proposed block. Once this consensus is achieved, the evidence is appended to the block, and the block is called sealed.

Sealed blocks are not automatically committed to the blockchain because they were formed against an old checkpoint, and the pool of actions laid down in the block must be revalidated. There can be duplicate or double-spending actions that must be disregarded.

There is no need for an additional consensus pool for this revalidation process. Every node does it on its own, and every honest node, starting from the same blocks, will arrive at the same conclusion. Namely, if the consensus is working, the revalidation can be performed independently.

Every node appends the list of hashes of invalidated actions in the revalidation process, signs it, and broadcasts it to exterior listeners (not to checksum committee nodes).

In case there is a problematic block for which consensus was not achieved, a sequence of sealed blocks will be formed against a stagnated checkpoint. If a certain number of these sealed blocks are found, the Swell protocol dictates that the block following this stagnated epoch should be declared nil (without any actions), so that the commit process can go on. This allows for reparations in mild deviations from consensus without requiring a full reset of the checksum window state.

### 1.3.3 Committee and P2P

Another departure from Tendermint is that not all validators participate at the same time in the consensus committee, and the messages from the consensus committee and the block broadcasting messages go through different network topologies.

Consensus committees are formed per block epoch and represent a small subset of the checksum validator pool. This increases the probability of occasionally forming malicious committees even when more than 2/3 of validators are honest. The fallback mechanism outlined above somehow mitigates the long-term consequences of such events. Also, contrary to Tendermint, only the leader in the consensus committee is allowed to propose a new block. If it fails to do so, honest nodes will try to settle consensus on a nil block.

Since the consensus committees are small and the exchanged messages are also small, the peer-to-peer network for the consensus is highly active, where every node broadcasts new messages it has received to every other node. This directs the communication channel and mitigates the effectiveness of malicious strategies that send incompatible messages to different nodes.

The block broadcast follows another strategy. First, unlike Tendermint, an honest node can vote for a hash even if it does not possess the block information that produces that hash. As long as more than 1/3 of voters claim to be in possession of the block, the honest node can trust that it will eventually be able to get that information.

In this sense, block broadcast is performed in a tiered scheme. The leader broadcasts blocks to some other nodes that will further broadcast this to additional nodes, and so on, until block information is percolated to the entire pool of checksum window validators. 

### 1.3.4 Candidate Nodes, Synchroinization and committee formation

Swell leverages the checksum windows strategy to address issues in decentralized proof-of-stake networks.

Firstly, since every node needs to clone the state at checksum points to enable rollback, they are also required to produce a standard checksum for that state.

Every candidate node aiming to become a validator in the next checksum window is asked to provide two rounds of evidence indicating possession of the correct state. Initially, they provide the checksum dressed (hashed with their own public key) and then naked. Candidates that timely provide compatible checksums in these two rounds, and have the permission to become a validator (for example, by depositing enough tokens in the proof-of-stake scheme), will be eligible to be selected in the next checksum window.

Finally, the list of all eligible nodes is hashed with the checksum to define the order of each node. If the number of eligible nodes exceeds the number of members in the committee, only the first nodes in the sorted list will be selected.

In principle, the node responsible for forming the last block associated with the checksum hash could skew the checksum in their favor. However, since the checksum operation is non-trivial, as long as the interval between blocks is short compared to the state complexity, this should not pose a serious risk to the protocol.

Checksums also elevate fast state sync strategies as first-class citizens in the crypto network. A node can obtain the prevailing state from another node without necessarily having to trust that node, as the network will provide a checksum against which the syncing node can attest to be in possession of the correct state.

This implies that there is no need for a node running the consensus layer to be in possession of the entire history of the blockchain. It only has to keep the state and recent blocks.

## 2 Networks

### 2.1 Cacimba do Padre Testnet

This is a general-purpose testnet running the Swell protocol under proof-of-authority permission, which can be used to test social protocols. The network is expected to be resilient. A gateway is available at the same address on port GGGG, and a block history database is also accessible at port BBBB. The default gateway will always "pay" for the processing fees in this testnet. If you still need fungible tokens, follow this [[link]].

### 2.2 Saquarema Testnet

This testnet, running the Swell protocol under proof-of-stake permission, is designed to test the Swell protocol itself and should not be relied upon to test social protocols. At least 2/3 of the stakes will run on honest nodes, and anyone is invited to perform malicious strategies with the remaining 1/3 to shut down the network or break consensus.

A validator node will be available as long as the network is functioning properly at port VVVV on the address XXXXX. A gateway will be available at port GGGG. There is no default block database in this network.

### 2.3 Build your own network

It is very easy to deploy a new network and the underlying infra-structure. To start a new validator node from a genesis state you have to provide a configuration file

(config file)

## 3 Nodes

### 3.1 Consensus node

In order to develop a social protocol, besides defining that state space, the menu of actions and the governing rules for validating actions, one has to implement the inner logic of breeze network.

Breeze groups blocks on consecutive checksum windows. The state at the last block of the previous 

### 3.2 Block database node

### 3.3 Gateway node

## 4 Developers

### 4.1 Design social protocols

### 4.2 Deploy social protocol as standalone validator

### 4.3 Standalone social protocol as a service

### 4.4 Build a social protocol consensus layer

#### 4.4.1 Using swell and a native token

#### 4.4.2 Using swell and token on a smart contract
