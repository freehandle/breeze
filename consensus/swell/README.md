SWELL
=====

Swell protocol defines the rules that a given set of validating nodes must follow
to mint a given number of blocks and define the new pool of validating nodes 
responsible for minting 

** Definition

A **token** is a public key associated to a Ed25518 Elliptic-curve cryptography

A **node** is a piece software running on a resolvable address that communicates 
with other nodes and (optionally) to external servers assciated to a token. 
All messages send from a node to its peers is signed against this token.

** PARAMETERS

```
W = Number of blocks minted within a checksum window
T = Time interval between blocks
P = Number of slots for nodes in the checksum window. A node can be granted more
    than one spot. W must be divisble by P
C = Numer of slots for nodes in a consesus pool. A noda can be granted more than
    one spot.
```

```
----+----+----+----+----+----+----+----+----+----+----+----+----+----+----+----+
              |                        |                   |    |
              checksum             checkpoint N            |    checksum 
              window N         checksum for the state      |    window N +1
                              at end of the checkpoint     | 
                              block must be calculated.    |
                                                           |
                                                    last block for
                                                      publishing 
                                                    naked checksum

```

Epoch 0 is the epoch of a genesis block that defines the initial state of the 
network.

The first checksum window 

