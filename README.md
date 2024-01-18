## Breeze

Protocol functionalities and usage are described troughout this document. 

[API Reference]([breeze command - github.com/freehandle/breeze - Go Packages](https://pkg.go.dev/github.com/freehandle/breeze)) 

Binary archives are published at [link]([GitHub - freehandle/breeze: High performance multipurpose crypto network designed for the development of digital interactions between people on the basiis of varied social protocols.](https://github.com/freehandle/breeze)).

Breeze actions include the transfer of tokens between accounts, the deposit of tokens as guarantee for participating in the consensus, the withdraw of tokens from consensus participants and a general purpose void action, to be used by more specialized protocols. 

When the void action is used, Breeze serves as a gateway for the processing of the underlying protocol's instructions.

<br/><br/>

## Building the source

For prerequisites and detailed build instructions please read the [installation instructions]().

Building blow requires a Go compiler (1.21 or later). You can install it using your favorite package manager. Once it is installed, run

**`make blow`**

Or, if you would rather build the full suit of utilities, run

**`make all`**

<br/><br/>

## Executables

Breeze works by use of four independent modules, each providing a different service. A brief description of each module and their respective requirements follows.

| Module     | Description                                                              |
| ---------- | ------------------------------------------------------------------------ |
| **`Blow`** | Creates a functioning node on a breeze network                           |
| **`Beat`** | Sets the action gateway service                                          |
| **`Echo`** | Performs the block storage and indexation service                        |
| **`Safe`** | Safekeeps keys and provides interaction interface for nodes and services |

<br/><br/>

## Modular architecture

Breeze was designed to provide three main services, uncoupled. 

1. The first service encompasses block creation and consensus. 
   
   There are two types of network connections regarding this service. One is a network connection with the remaining validation peers responsible for block generation in a given checksum window. The other is a relay for external communication, so that actions can be received and block events can be sent. 
   
   The external connection will usually be brokered, as opposed to keeping an open port for any connection request.

2. The second service encompasses block storage and indexing.
   
   It is responsible for listening for new blocks and storing them. It also indexes block information and broadcasts them for external requests.

3. The third and last service provides a gateway for validator nodes. It keeps active nodes connected and manages the fowarding of actions for the nodes most likely to incorporate them into a new block.

With these three services, and given the void action prescribed by the breeze protocol, it is possible to also provide more specialized protocols as a forth service. Social protocols can be designed for specific uses and easily deployed as a forth decoupled service on top of the breeze network.

<br/><br/>

## Hardware requirements for running any module

Minimum:

- CPU with 4 cores

- 8Gb RAM

- 20 MBit/sec download interned service

<br/><br/>

## Safe module overview

Safe module provides key safekeeping, and commands for interaction with nodes and the other services.  Main safe commands and their respective description follow

- **`create`**
  
  Creates a new secure vaut with a random crypto key

- **`show`**
  
  Shows information about the vault file

- **`sync`**
  
  Connects to trusted node and asks for the secret keys the node expects to receive

- **`register`**
  
  Registers a new trusted node on the breeze network

- **`grant`**
  
  Grants the token access to connect to the trusted node as a gateway or block listener

A full list of safe module commands can be found in a later [section](#safe-full-command-list) of this document, including more detailed descriptions.

<br/>

#### Running safe

To run safe, 

**`safe <path-to-vault-file> <command> [arguments]`**



To check any of the commands help instructions, run

**`go run ./safe help <command>`** replacing the `<command>` with the command you wish to look for informations of.



To start using you may create a vault. From within the `cmd` folder, run

**`go run ./safe path/to_vault/vault_name create`**

To which you will be asked to provide a pass phrase, the phrase provided will be used to encrypt the vault. Once the safe is generated, the remaining commands may be used to perform various functionalities on the breeze network. The functionalities associated with each module will be reference on each module's topic throughout this document.

<br/><br/> 

## Running blow

Blow module can run on two different scenarios: either proof-of-stake permission, or proof-of-authority. 

#### Proof-of-stake permission configuration

Node can run on the official testnet or you may create a new network from genesis. 

1. To run on the official testnet
   
   <br>

2. To create a new network from genesis, create a json file within the blow folder. The file must include the following fields, as explained in the example. All tokens are in hex string format. 

```
{
    "token" : "public key associated with node owner",
    "address": "address associated with the node. may be either an IP or domain name",
    "adminPort": port for admin connections. 5403 for standard breeze configuration,
    "walletPath": "left empty for memory based wallet store. if filled, must be a path to valid folder with appropriate permissions",
    "logPath": "left empty for standard logging. if filled, must be a path to a valid folder with appropriate permissions",
    "network": // optional field for network preferences, left empty for PoS standard breeze configuration 
    {
        "breeze": { breeze network configuration (see below), left empty for standard breeze configuration },
        "permission":  { permission configuration (see below), left empty for standard breeze PoS } 
    },
    "relay": {
        "gateway": {
            "port": gateway port, 5404 for standard breeze configuration,
            "throughput": number of actions per block to be forwarded,
            "firewall": { firewall configuration (see below) }
        },
        "blockStorage": {
            "port": block events broadcasting port, 5405 for standard breeze configuration,
            "storagePath": valid directory for block database files persistent storage,
            "indexWallets": true if actions should be indexed, false otherwise,
            "firewall": { firewall configuration (see below) }
        },
    }
}
```

<br/>

If you choose not to use Breeze's standard configuration, you may provide the "breeze" json field the following setup:

```
"breeze": {
    "gossipPort": port for broadcasting. 5401 for standard breeze configuration,
    "blocksPort": port for broadcasting blocks 5402, for standard breeze configuration,
    "blockInterval": interval between block formation. 1000 for standard breeze configuration,
    "checksumWindowBlocks": number of blocks per checksum window, 900 for standard breeze configuration,
    "checksumCommitteeSize": number of participants in consensus commitee. 100 for standard breeze configuration,
    "maxBlockSize": block size limit. 100000000 for standard breeze configuration. must be at least 1MB,
    "swell" : // configuring swell protocol credentials
    {
        "committeeSize": number of participant nodes in swell consensus committee. 10 for standard breeze configuration,
        "proposeTimeout": timeout limit for a proposal for the hash of the block, in milliseconds. 1500 for standard breeze configuration,
        "voteTimeout": timeout limit to wait in vote state, in milliseconds. 1000 for standard breeze configuration,
        "commitTimeout": timeout limit to wai in commit state, in milliseconds. 1000 for standard breeze configuration,
    },
},
```

<br/>

By choosing the standard Proof-of-Stake permission configuration, you may provide the "permission" json field the following setup:

```
"permission": {
    "pos": // proof-of-stake permission configuration, as opposed to standard "poa" (proof-of-authority)
    {
        "minimimStake": minimum amount of tokens deposited for a node to be    eligible for the committee. 1e6 for standard breeze configuration,
    },
},
```

<br/>

For the firewall configuration, you may provide the "firewall" json field the following setup:

```
 "firewall": {
     "openRelay": if true, relay is left open for any external connection. if false, firewall blocks connections outside listed permissions,
     "whitelist": when openRelay is false, this field must provide a list of permitted connections as in [ "token1", "token2", ... ] format,
 }
```

For a functioning example of this json file, please refer to [json_example]().

Once you have the file filled with the desired configuration, run

 **`./blow breeze_pos.json genesis`**

<br/>

#### Proof-of-authority scenario

You may run a network under the proof-of-authority permissioning protocol. In this case, you will need to provide trusted nodes' adresses. This can only be performed by the initial authority token. The trusted nodes list can be posteriorly updated by the admin console. 

In order to choose the Proof-of-Authority permission configuration, you must provide the "permission" json field the following setup:

```
"permission": {
    "poa": // proof-of-authority permission configuration, as opposed to standard "pos" (proof-of-stake)
    {
        "trustedNodes": list of trusted node addresses as in ["token1", "token2", ...]
    },
},
```

<br/>

#### Using safe to manage node

Some safe commands deal with blow module functionalities. These are listed on the following table

| Command        | Description                                                                                                                                                                                                                                                                                                                                                        |
| -------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **`status`**   | Provides a given node's status on the network.                                                                                                                                                                                                                                                                                                                     |
| **`sync`**     | Connects to trusted node and asks for the secret keys the node expects to receive. The trusted node will only accept the connection if the token associated to the secret key of the vault is configured with admin rights on the trusted node.                                                                                                                    |
| **`register`** | Registers a new trusted node on the breeze network. The node ID is a unique identifier for the node within the vault. The command also requires an address, which mus be a valid TCP address, a token which mus be the token associated to the node and a human readable description of the node. The token is used to authenticate signed connection to the node. |
| **`remove`**   | Removes associated node from the pool of trusted nodes within the vault. The action will only have effect if the provided node ID is already registered within the vault.                                                                                                                                                                                          |
| **`nodes`**    | Lists all the trusted nodes registered within the vault.                                                                                                                                                                                                                                                                                                           |
| **`transfer`** | Instructs node to transfer a given amount of funds from a given account to the pointed account.                                                                                                                                                                                                                                                                    |
| **`deposit`**  | Instructs node to deposit a given amount of funds from a given account to the pointed account.                                                                                                                                                                                                                                                                     |
| **`withdraw`** | Instructs node to withdraw a given amount of funds from a given account.                                                                                                                                                                                                                                                                                           |
| **`balance`**  | Returns the balance of a given account.                                                                                                                                                                                                                                                                                                                            |
| **`config`**   | Configures a given variable's value for the node.                                                                                                                                                                                                                                                                                                                  |
| **`activity`** | Instructs trusted node whether to candidate to become a validator. It will only have effect in the next checksum window. To shutdown a node immediately connect to the server running the node. The trusted node will only accept the connection if the token associated to the secret key of the vault is configured with admin rights on the trusted node.       |

<br/>

## Running beat

Run

**`beat <config.json>`**

The config.json file provided must include the following fields

<br/>

#### Beat Configuration

Beat module configuration takes the following information, with token adresses in hex string format

```
// check
"token": token,
"wallet": address of the wallet 
"port": broadcasting port 
"adminPort": port for admin connections

"logPath": left empty for standard logging. if filled, should be a path to folder
"actionRelayPort": Port actions transmissions
"blockRelayPath": Port for block transmissions 
"breeze": Breeze network configuration. Left empty for standard POS configuration
"firewall":Firewal configuration
"trusted": List of trusted nodes to connect when not participating in the validator pool
```

#### 

#### Admin beat module with safe

As presented on the Safe module topic above, some safe commands deal with beat module functionalities. These are listed on the following table

| Command        | Description                                                                                                                                                                                                                                                                                                                                      |
| -------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **`grant`**    | Grants the token access to connect to the trusted node as a gateway or block listener. The trusted node will only accept the connection if the token associated to the secret key of the vault is configured with admin rights on the trusted node.                                                                                              |
| **`revoke`**   | Revokes the token's connect access to the trusted node as a gateway or block listener. The trusted node will only revoke the connection if the token associated to the secret key of the vault is configured with admin rights on the trusted node. The action will only have effect if the token is already granted access to the trusted node. |
| **`firewall`** | Configures firewall functionality for a given port.                                                                                                                                                                                                                                                                                              |

<br/>

## Running echo

Run

**`echo <path-to-json-config-file>`**

The config.json file provided must include the following fields

<br/>

#### Echo Configuration

Echo module configuration takes the following information

```
// check
"token": Token of the service,
"port": Port for service providing,
"address": Node address (IP or domain name),
"adminPort": Port for admin connections,
"storagePath": Left empty for standard storage. If filled, should be a path to folder,
"indexed": Boolean, true for indexing of blocks adresses. false otherwise,
"walletPath": Left empty for memory based wallet store. If filled, must be a path to folder,
"logPath": Left empty for standard logging, if filled, should be a path to folder,
"breeze": breeze network configuration. left empty for standard POS configuration,
"firewall": firewall configuration,
"trusted": list of trusted nodes to connect when not participating in the validator pool,
```

<br/>

#### Admin echo module with safe

As presented on the Safe module topic above, some safe commands deal with echo module functionalities. These are listed on the following table

| Command        | Description                                         |
| -------------- | --------------------------------------------------- |
|                |                                                     |
|                |                                                     |
|                |                                                     |
| **`firewall`** | Configures firewall functionality for a given port. |

<br/>

## Contribution

#### Synergy

[Synergy]([GitHub - freehandle/synergy: Social Protocol for the exploration of collective action into building a personal internet](https://github.com/freehandle/synergy)) protocol was designed as a digital framework for collaboration and collective construction. It works seamlessly on top of the Breeze protocol working with  

[handles]([GitHub - freehandle/axe: Basic ID layer with delegation for the social funcionalities within breeze network](https://github.com/freehandle/axe)) social protocol, which provides  primitives for identity and stage management.

Breeze is, itself, an ongoing project inside the Synergy protocol. To collaborate with building Breeze, you may join [Synergy's Breeze Collective](). 

<br/><br/>

## Safe full command list

Safe module's full list of commands and their respective description follow. 

| Command        | Description                                                                                                                                                                                                                                                                                                                                                        |
| -------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **`create`**   | Creates a new secure vault with a random crypto key. The vault will be encrypted with a password provided by the user.                                                                                                                                                                                                                                             |
| **`show`**     | Shows information about the vault file.                                                                                                                                                                                                                                                                                                                            |
| **`status`**   | Provides a given node's status on the network.                                                                                                                                                                                                                                                                                                                     |
| **`sync`**     | Connects to trusted node and asks for the secret keys the node expects to receive. The trusted node will only accept the connection if the token associated to the secret key of the vault is configured with admin rights on the trusted node.                                                                                                                    |
| **`register`** | Registers a new trusted node on the breeze network. The node ID is a unique identifier for the node within the vault. The command also requires an address, which mus be a valid TCP address, a token which mus be the token associated to the node and a human readable description of the node. The token is used to authenticate signed connection to the node. |
| **`remove`**   | Removes associated node from the pool of trusted nodes within the vault. The action will only have effect if the provided node ID is already registered within the vault.                                                                                                                                                                                          |
| **`nodes`**    | Lists all the trusted nodes registered within the vault.                                                                                                                                                                                                                                                                                                           |
| **`transfer`** | Instructs node to transfer a given amount of funds from a given account to the pointed account.                                                                                                                                                                                                                                                                    |
| **`deposit`**  | Instructs node to deposit a given amount of funds from a given account to the pointed account.                                                                                                                                                                                                                                                                     |
| **`withdraw`** | Instructs node to withdraw a given amount of funds from a given account.                                                                                                                                                                                                                                                                                           |
| **`balance`**  | Returns the balance of a given account.                                                                                                                                                                                                                                                                                                                            |
| **`config`**   | Configures a given variable's value for the node.                                                                                                                                                                                                                                                                                                                  |
| **`grant`**    | Grants the token access to connect to the trusted node as a gateway or block listener. The trusted node will only accept the connection if the token associated to the secret key of the vault is configured with admin rights on the trusted node.                                                                                                                |
| **`revoke`**   | Revokes the token's connect access to the trusted node as a gateway or block listener. The trusted node will only revoke the connection if the token associated to the secret key of the vault is configured with admin rights on the trusted node. The action will only have effect if the token is already granted access to the trusted node.                   |
| **`activity`** | Instructs trusted node whether to candidate to become a validator. It will only have effect in the next checksum window. To shutdown a node immediately connect to the server running the node. The trusted node will only accept the connection if the token associated to the secret key of the vault is configured with admin rights on the trusted node.       |
| **`generate`** | Generates a random ED25519 cryptographic key-pair and stores the private key on the secure vault file. The public key is printed to the standard output.                                                                                                                                                                                                           |
| **`firewall`** | Configures firewall functionality for a given port.                                                                                                                                                                                                                                                                                                                |

<br/> <br/>

## License

Breeze is licensed under the [Apache 2.0 license](https://www.apache.org/licenses/LICENSE-2.0.txt). 
