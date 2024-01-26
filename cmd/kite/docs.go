package main

import "fmt"

const helpShow = `usage: kite <path-tovault-file> show

Show information about the vault file.
`

const helpCreate = `usage: kite <path-tovault-file> create

Create a new secure vault with a random crypto key. The vault will be encrypted
with a password provided by the user. 
`

const helpRegister = `usage: kite <path-tovault-file> register <node-id> <address> <token> <description> 

Register a new trusted node on the breeze network. The node-id is a unique
identifier for the node within the vault. The address a valid TCP address, token 
is the token associated to the node and description is a human readable 
description of the node. The token is used to authenticate signed connection to 
the node.
`

const helpRemove = `usage: kite <path-tovault-file> remove <node-id> 

Remove the associated node from the pool of trusted nodes within the vault. 
The action will only hjave effect if the provided node-id is already registered
within the vault.
`

const helpNodes = `usage: kite <path-tovault-file> nodes

List all the trusted nodes registered within the vault.
`

const helpGenerate = `usage: kite <path-tovault-file> generate id description

New generates a random ED25519 cryptographic key-pair and store the private
key on the secure vault file. The public key is printed to the standard output.

`

const helpImport = `usage: kite <path-to-pem-file> import id description

Import an existing ED25519 private key from a pem file. The public key is 
printed to the standard output.
`

const helpSync = `usage: kite <path-tovault-file> sync <node-address> <node-temp-token>

Will connect to trusted node and ask for the secrets keys the node is expecting
to receive. The trusted node will only accept the connection if the token 
associated to the secret key of the vault is configured with admin rights on the 
trusted node.
The <node-address> parameter has the DNS:port format
The <node-temp-token> is the ephemeral token provided by the node 
`

const helpGrant = `usage: kite <path-tovault-file> grant <node-id> <token> [gateway|block] [description]

Will grant the token access to connect to the trusted node as a gateway or block listener. 
The trusted node will only accept the connection if the token 
associated to the secret key of the vault is configured with admin rights on the 
trusted node.
`

const helpRevoke = `usage: kite <path-tovault-file> revoke <node-id> <token> [gateway|block]

Will revoke the token connect access to the trusted node as a gateway or block 
listener. The trusted node will only accept the connection if the token  
associated to the secret key of the vault is configured with admin rights on the 
trusted node. The action will only have effect if the token is already granted
access to the trusted node.
`

const helpActivity = `usage: kite <path-tovault-file> activity <node-id> [activate|deactivate]

Will instruct the trusted node whether to candidate to become a validator. It
will only have effect in the next checksum window. To shutdown a node immediately
connect to the server running the node. 

The trusted node will only accept the connection if the token  
associated to the secret key of the vault is configured with admin rights on the 
trusted node.
`

const helpTransfer = `usage: kite <path-tovault-file> transfer <from-account> <to-account> <ammount> <fee> 

Will instruct node to transfer token-amount of funds from from-account to to-account.
`

const helpDeposit = `usage: kite <path-tovault-file> deposit <token-amount> <account>

Will instruct node to deposit token-amount of funds to given account.
`

const helpWithdraw = `usage: kite <path-tovault-file> withdraw <token-amount> <account>

Will instruct node to withdraw token-amount of funds from given account.
`

const helpBalance = `usage: kite <path-tovault-file> balance <account>

Will instruct node to 
`

const helpConfig = `usage: kite <path-tovault-file> config <variable> <node-id>

Will instruct .
`

func help(cmd string) {
	switch cmd {
	case "create":
		fmt.Print(helpCreate)
	case "register":
		fmt.Print(helpRegister)
	case "remove":
		fmt.Print(helpRemove)
	case "nodes":
		fmt.Print(helpNodes)
	case "generate":
		fmt.Print(helpGenerate)
	case "sync":
		fmt.Print(helpSync)
	case "grant":
		fmt.Print(helpGrant)
	case "revoke":
		fmt.Print(helpRevoke)
	case "activity":
		fmt.Print(helpActivity)
	case "transfer":
		fmt.Print(helpTransfer)
	case "deposit":
		fmt.Print(helpDeposit)
	case "withdraw":
		fmt.Print(helpWithdraw)
	case "balance":
		fmt.Print(helpBalance)
	case "config":
		fmt.Print(helpConfig)
	default:
		fmt.Print(usage)
	}

}
