## Breeze

Official implementation of the breeze protocol and associated utilities.

For a description of the breeze protocol see [breeze presentation](https://github.com/freehandle/breeze/blob/main/breezedoc.md).

This file is about running breeze network infrastructure. For instructions about deploying specialized protocols on top of breeze network please refer to [social protocol documentation](https://github.com/freehandle/breeze/middleware/social/README.md).

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

Kite module is used for remote administration of modules and to send actions to 
breeze network. 

Basic usage:

To create a new vault for secrets safekeeping

```
kite <file-name-for-new-vault> create 
```

To show information about the vault, incluind public key associated to the vault


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

Where <node-id> is used to refer to the node in the administration commands. For example in order to grant/revoke tokens access to node functionalities 

```
kite <path-to-exisitng-vault-file> [grant|revoke] <node-id> <token> [gateway|block] (description)
```

Detailed information about these and other funcionalities can be found in the kite help command.

```
kite help
```


## Running blow

To run blow validator one has to provide a json configuration file with the desired specifications.

```
blow <path-to-json-config-file>
```

The simplest scenario to run blow is as a validator candidate for the proof-of-stake Paúba testnet. 
Check [freehandle.org](freehandle.org/testnets) to get instructions on how to get necessary tokens to stake for permission. 

In the configurations __Public Keys__ are always represented by its hexadecimal 64-char representation without any prefix. The network relies on token-based firewall rules. Firewall configuration is of the form

```
{
    "open": [true|false]
    "tokenList": [<token 1>, <token 2>,...] 
 }
```

When "open" is set to __true__ the firewall will by default allow all connections except those blacklisted by the "tokenList". When __false__ the firewall will by default forbid all connections except those whitelisted by the "tokenList". 

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

The underlying system must keep the ports 5401, 5402, 5404 and 5405 open for tcp connections from anywhere. Even though not required by the protocol, it is desirable that validator nodes keep gateway and blocks relay firewalls open so that gateway services and block listeners can connect to the validator.

One can check [freehandle.org](freehandle.org/testnets/pauba) for a freehandle trusted node for the Paúba proof-of-stake testnet.

After running the node one has to use kite to sync the secret key associated with the node token. The token must be a public key indexed in the vault file. 

The service will try to connect to trusted nodes to sync state and if successfull candidate to become a validator. 

#### Personalized breeze configuration

In order to configure a personalized breeze network one has to provide more detailed information. Besides providing the information in the configuration above (possibly with other ports), one has to provide information about the network. First one has to define the permission schema and the breeze parameter in the form. 

```
{
    ... (root cofig as above) ...
    "network": {
        "permission": { permission config },
        "breeze" : { breeze config },
    }
}
```

For the permission config it can be a proof-of-authority

```
{
    "poa": {
        "trustedNodes": list of trusted node addresses as in ["token1", "token2", ...]
    }
}
```

In this case, only nodes with the secret keys associated with the tokens can candidate to become validators. Alternatively, one can define a proof-of-stake permission

```
{
    "pos": {
        "minimimStake": minimum amount of tokens required for elibigility
    },
}
```

where anyone with __minimumStake__ deposited is elebigible to candidate for a validator.

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

If the network is starting from genesis its parameters creating the __aero__ fungible tokens and their initial distribution must also be specified: 

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

<br/>

One can check the Itamambuca testnet [configuration]() for a comprehensive example.

Whenever genesis is specified blow will initiate a new blockchain from scratch. When not specified blow will look for state syncrhonization from trusted nodes. If neither genesis nor trusted nodes are specified blow will terminate with an error.

## Running beat

Beat gateway can be executed linking to a beat config file:

```
beat <path-to-beat-config.json>
```

#### Beat Configuration

Basic configutation for a beat gateway on a standard breeze network (both Paúba and Itamambuca testnets) are:

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

Gateway will try to connect to trusted nodes to receive information about the current pool of validators and connect to them to provide gateway funcionality. The firewall rule specifices who can connect to the beat node on the specified "port". 

In case beat is used to route action for a non standard breeze network an aditional field "breeze" must be specified according to the prescription of the blow module above. 

In case the gateway offers the service to pay for clearing fees in the network an additional wallet field must be specified. If specified beat will dress all received actions with its wallet and pay its perceived market rate for fees (algorithm not yet implemented). 

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

Basic configutation for an echo storage service on a standard breeze network (both Paúba and Itamambuca testnets) are:

```
{
    "token": <node token>,
    "port": 5420, 
    "adminPort": 5423,
    "logPath": "empty for standard logging, or path to folder for file logging",
    "storagePath": "path to a folder to save block history and its indexes",
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

If indexed is set to false, it will only serve as a block storage and will only provide entire blocks. If indexed is set to true, it will index actions by token and can send action history associated to those tokens. 

Echo will connect to trusted nodes to receive information about the current pool of validators and connect to them to receive new blocks from them. 

(TODO: block history from other echo nodes)

Like blow and beat, after running echo with the configuration file, kite must be used to share secret keys associated with the echo node token. 

## Contribution

#### Synergy

[Synergy](https://github.com/freehandle/synergy) protocol was designed as a digital framework for collaboration and collective construction. It runs seamlessly on top of the Breeze protocol working with  

[Handles](https://github.com/freehandle/axe) social protocol, which provides primitives for identity and stage management.

Breeze is, itself, an ongoing project inside the Synergy protocol. To collaborate with building Breeze, you may join [Synergy's Breeze Collective](https://freehandle.org/synergy/collective/synergy). 

#### Github

The freehandle sponsored implementation of the breeze protocol and the primitive social protocols will be developed on the [freehandle](https://github.com/freehandle) repositories on github. 




## License

Breeze is licensed under the [Apache 2.0 license](https://www.apache.org/licenses/LICENSE-2.0.txt). 
