## Breeze

Official implementation of the breeze protocol and associated utilities.

For a description of the breeze protocol see [breeze presentation](https://github.com/freehandle/breeze/blob/main/breezedoc.md).

This file deals with running breeze network infrastructure. For instructions about deploying specialized protocols on top of breeze network please refer to [social protocol documentation](https://github.com/freehandle/breeze/middleware/social/README.md).

## Building the source

Building blow requires a Go compiler (1.21 or later). You can install it using your favorite package manager. Once it is installed, run

**`make all`**

to build all executables within cmd folder. Otherwise it is possible to compile one by one using standard go toolchain procedures.  

## Executables

Breeze network usage relies on four independent services, found in cmd folder, each providing a specific functionality. 

| Module     | Description                                                                  |
| ---------- | ---------------------------------------------------------------------------- |
| **`blow`** | sequencer and validator for the breeze protocol                              |
| **`beat`** | gateway that receives actions (transactions) and forwards them to validators |
| **`echo`** | block storage and indexing                                                   |
| **`kite`** | remote administration of services and safekeeping of crypto secrets          |

## Modular architecture

Breeze is designed to provide three main services, uncoupled. 

1. The first service encompasses block creation and consensus. 
   
   There are two types of network connections regarding this service. One is a network connection with the remaining validation peers responsible for block generation in a given checksum window. The other is a relay for external communication, so that actions can be received and block events can be sent. 
   
   The external connection will usually be brokered, as opposed to keeping an open port for any connection request.

2. The second service encompasses block storage and indexing.
   
   It is responsible for listening for new blocks and storing them. It also indexes block information and broadcasts them for external requests.

3. The third and last service provides a gateway for validator nodes. It keeps active nodes connected and manages the fowarding of actions for the nodes most likely to incorporate them into a new block.

With these three services, and given the void action prescribed by the breeze protocol, it is possible to also provide more specialized protocols as a forth service. Social protocols can be designed for specific uses and easily deployed as a forth decoupled service on top of the breeze network.

## Minimum hardware requirements for running each module

#### blow/beat:

- CPU with 2 cores

- 4Gb RAM

- 20 MBit/sec internet connectivity

- static IP address

#### echo:

- CPU with 4 cores

- 16Gb RAM

- 20 MBit/sec internet connectivity

- 1Tb disk space 

#### kite:

- any configuration

## Kite module overview

Kite module is used for remote administration of modules and to send actions to breeze network. 

Basic usage:

To create a new vault for secrets safekeeping

```
kite <file-name-for-new-vault> create 
```

To show information about the vault, including the public key associated with the vault

```
kite <path-to-existing-vault-file> show
```

To create a new cryptographic key pair

```
kite <path-to-existing-vault-file> generate
```

The public key will be shown.

To share secrets with remote module 

```
kite <path-to-exisitng-vault-file> sync <remote-address> <remote-token>
```

Before using kite for remote administration of modules one has to register them as trusted nodes

```
kite <path-to-exisitng-vault-file> register <node-id> <address> <token> <description>
```

Where <node-id> is used to refer to the node in the administration commands. For example, in order to grant/revoke tokens access to node functionalities 

```
kite <path-to-exisitng-vault-file> [grant|revoke] <node-id> <token> [gateway|block] (description)
```

Detailed information about these and other funcionalities can be found through the kite help command.

```
kite help
```

#### Token management

Each of the blow, beat and echo modules must be provided with an associated configuration file upon deployment of module's instance. A running instance of any of these modules will be referred as node. 

Token management and storage are dealt with by a vault that can be both generated and managed by kite module. Previous to deploying a node, it's associated vault can be created from kite

```
kite <file-name-for-new-vault> create
```

The created vault's public token can be then be checked

```
kite <file-name-for-new-vault> show
```

The configuration file for each module's node include a field "token" which will be filled with the token associated with the node. This field must match node's vault public key. 

In order to sync to a running node, kite module is used from vault owner

```
kite <path-tovault-file> sync <node-address> <node-ephemeral-token>
```

Node's address must be provided in the DNS:port format and node's ephemeral token is the token automatically generated by upon deplying the node with the config file.


## Running blow

To run blow validator one has to provide a json configuration file with the desired specifications.

```
blow <path-to-json-config-file>
```

The simplest scenario to run blow is as a validator candidate for the proof-of-stake Paúba testnet. 
Check [freehandle.org](freehandle.org/testnets) to get instructions on how to get necessary tokens to stake for permission. 

In the configuration file, __Public Keys__ are always provived in their hexadecimal 64-char representation without any prefix. The network relies on token-based firewall rules. Firewall configuration is of the form

```
{
    "open": [true|false]
    "tokenList": [<token 1>, <token 2>,...] 
 }
```

When "open" is set to __true__ the firewall will by default allow all connections except those blacklisted by the "tokenList". When __false__, the firewall will by default forbid all connections except those whitelisted by the "tokenList". 

#### Proof-of-Stake standard configuration

```
{
    "token" : "node public key",
    "address": "node address: may be either an IP or domain name",
    "adminPort": 5403, 
    "walletPath": "empty (for memory only) or a path to folder (for persistence)",
    "logPath": "empty (for standard logging) or a path to log folder",
    "relay": {
        "gateway": {
            "port": 5404,
            "throughput": 15000,
            "maxConnections" : <any number of connections>,
            "firewall": { firewall configuration (see above) }
        },
        blocks": {
            "port": 5405,
            "maxConnections" : <any number of connections>,
            "firewall": { firewall configuration (see above) }
        },
    },
    "trustedNodes": [
        {
            "address": "trusted node address (without port)",
            "token": <trusted node token>
        },...
    ]
}
```

The underlying system must keep the ports 5401, 5402, 5404 and 5405 open for TCP connections from anywhere. Although not required by the protocol, it is desirable that validator nodes keep gateway and blocks relay firewalls open so that gateway services and block listeners can connect to the validator.

One can check [freehandle.org](freehandle.org/testnets/pauba) for a freehandle trusted node for the Paúba proof-of-stake testnet.

After running the node one has to use kite to sync the secret key associated with the node token. The token must be a public key indexed in the vault file. 

The service will try to connect to trusted nodes to sync state and, if successfull, candidate to become a validator. 

#### Personalized breeze configuration

In order to configure a personalized breeze network more detailed information must be provided. Besides the information in the configuration above (possibly with other ports), information about the network must also be provided. First step is defining the permission schema and the breeze parameter in the form: 

```
{
    ... (root cofig as above) ...
    "network": {
        "permission": { permission config },
        "breeze" : { breeze config },
    }
}
```

The permission config can be a proof-of-authority

```
{
    "poa": {
        "trustedNodes": list of trusted node addresses as in ["token1", "token2", ...]
    }
}
```

In this case, only nodes with the secret keys associated with the tokens can candidate to become validators. 

Alternatively, permission configuration can be proof-of-stake

```
{
    "pos": {
        "minimimStake": minimum amount of tokens required for elibigility
    },
}
```

where anyone providing a __minimumStake__ deposit is elebigible to candidate for a validator.

If the permission field is left empty the network will be permissionless, and anyone can candidate to become a validator. 

With respect to the breeze configuration there are several paramenters to be defined:

```
"breeze": {
    "gossipPort": <port for consensus voting: 5401 for standard>,
    "blocksPort": <port for broadcasting blocks: 5402 for standard>,
    "blockInterval": <time interval (in Milisseconds) between blocks: 1000 for standard>,
    "checksumWindowBlocks": <number of blocks per checksum window, 900 for standard>,
    "checksumCommitteeSize": <number of participants in consensus commitee: 100 for standard>,
    "maxBlockSize": <block size limit. 100000000 for standard>,
    "swell" : {
        "committeeSize": <participants in swell consensus committee: 10 for standard>,
        "proposeTimeout": <in milliseconds: 1500 for standard>,
        "voteTimeout": <in milliseconds: 1000 for standard>,
        "commitTimeout": <in milliseconds: 1000 for standard>,
    },
},
```

Significance of swell parameters can be found in the [swell algorithm specification](/consensus/bft/README.md). 

If starting from genesis network parameters for creating the __aero__ fungible tokens and their initial distribution must also be specified: 

```
    ... (root config) ...
    "genesis" : {
        "wallets": [
            {
                "token": <wallet token>,
                "wallet": <number of aero credited>,
                "deposit": <number of aero deposited>,
            }, ...
        ],
        "networkID": "any string"
    }
```

Refer to the Itamambuca testnet [configuration]() for a comprehensive example.

Whenever genesis is specified, blow will initiate a new blockchain from scratch. When not specified, blow will look for state synchronization from trusted nodes. If neither genesis nor trusted nodes are specified, blow will terminate with an error.

## Running beat

Beat gateway can be executed linking to a beat config file:

```
beat <path-to-beat-config.json>
```

#### Beat Configuration

Basic configuration for a beat gateway on a standard breeze network (both Paúba and Itamambuca testnets) is of the form:

```
{
    "token": <node token>,
    "port": 5410, 
    "adminPort": 5413,
    "logPath": "empty for standard logging, or path to folder for file logging",
    "actionRelayPort": 5404,
    "blockRelayPath": 5405, 
    "firewall": { node firewall configuration },
    "trustedNodes": [
        {
            "address": "trusted node address (without port)",
            "token": <trusted node token>
        }, ...
    ]
}
```

Gateway will try to connect to trusted nodes to receive information about the current pool of validators and connect to them to provide gateway functionality. The firewall rule specifies who can connect to the beat node on the "port" appointed. 

In case beat is used to route action for a non standard breeze network, an aditional field "breeze" must be specified according to the prescription of the blow module above. 

In case the gateway offers the service to pay for clearing fees in the network, an additional wallet field must be specified. When specified, beat will dress all received actions with its wallet and pay its perceived market rate for fees (algorithm not yet implemented). 

```
{
    ... (as above) ...
    "wallet": <wallet token>,
}
```

Like blow, after running beat with the configuration file, kite must be used to share secret keys associated with the node token and wallet token. 

## Running echo

Echo block storage service can be executed linking to an echo config file:

**`echo <path-to-echo-config.json>`**

#### Echo Configuration

Basic configuration for an echo storage service on a standard breeze network (both Paúba and Itamambuca testnets) is of the form:

```
{
    "token": <node token>,
    "port": 5420, 
    "adminPort": 5423,
    "logPath": "empty for standard logging, or path to folder for file logging",
    "storagePath": "path to folder to save block history and its indexes",
    "indexed": true,
    "blocksPort": 5405,
    "firewall": { node firewall configuration },
    "trustedNodes": [
        {
            "address": "trusted node address (without port)",
            "token": <trusted node token>
        }, ...
    ]
}
```

If "indexed" is set to false, it will serve as a block storage and providing only entire blocks. If "indexed" is set to true, it will index actions by token and can send action history associated to referred tokens. 

Echo will connect to trusted nodes to receive information about the current pool of validators and connect to them to receive new blocks from them. 

(TODO: block history from other echo nodes)

Like blow and beat, after running echo with the configuration file, kite must be used to share secret keys associated with the echo node token. 

## Contribution

#### Synergy

[Synergy](https://github.com/freehandle/synergy) protocol was designed as a digital framework for collaboration and collective construction. It runs seamlessly on top of the Breeze protocol working with  

[Handles](https://github.com/freehandle/handles) social protocol, which provides primitives for identity and stage management.

Breeze is, itself, an ongoing project inside the Synergy protocol. To collaborate with building Breeze, you are welcome to join [Synergy's Breeze Collective](https://freehandle.org/synergy/collective/synergy). 

#### Github

The freehandle sponsored implementation of the breeze protocol and the primitive social protocols will be developed on the [freehandle](https://github.com/freehandle) repositories on github. Everyone is welcome to participate in improving these implementations.  

Contributions that **do not** change protocol functionalities, such as bug fixes, testing coverage, code refactorings, improving middleware utilities, etc, may be proposed directly as a pull request targeting the main branch of [Breeze official repository](). 

For such contributions, please follow these steps:

1. [Fork]([Fork a repository - GitHub Docs](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/working-with-forks/fork-a-repo)) Breeze's official repository to your github profile 

2. [Clone]([Cloning a repository - GitHub Docs](https://docs.github.com/en/repositories/creating-and-managing-repositories/cloning-a-repository)) the forked repository in your local PC 

3. Implemente the changes locally

4. [Push]([Git Guides - git push · GitHub](https://github.com/git-guides/git-push)) commited changes to your remote repository

5. Issue a [Pull Request]([Creating a pull request - GitHub Docs](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/proposing-changes-to-your-work-with-pull-requests/creating-a-pull-request)) targeting Breeze's official repository

For contributions that in anyway include protocol change, please join [Synergy's Breeze Collective]() and join a previous discussion involving the community, so decisions regarding the changes can be made collectively. 

## License

Breeze is licensed under the [Apache 2.0 license](https://www.apache.org/licenses/LICENSE-2.0.txt). 
