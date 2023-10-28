# Swell

Swell is a slight variant of tendermint consensus algorithm, designed to reduce latency and improve trouhghput of the network.
Contrary to tendermint, consensus committee for diferent epochs (heights in tendermint nomenclature) are independent. A consensus for block of a given epoch can be achieved before the consensus for a block of a prior epoch. 

Blocks are proposed against a given checkpoint epoch, for which state the incorporated actions are validated. The formation of the blockchain and the-revalidation of each action against the state of the previous block is done independently by each node. 

This means that consensus is generated only on the hash of the actions for the block (plus header details). The final block, with
revalidation is not subject to consensus. Nonetheless each honest node will perform excatly the same actions to revalidate thus 
providing the same information.

Byzantine Fault Tolerant (BFT) consensus algorithm typically involves a sequence of rounds, each one with a designated leader that can propose a value for the committee. For swell this value is alwas a hash, and contrary to tendermint honest nodes do not have
the freedom to propose values of their own in subsequent rounds. Only the leader of the first round is allowed to propose a value
(and broadcast the block header + actions information to its peers). If consensus is not achieved in the first round, honest nodes
will either keep repeting the value receved from the leader of the first round or propose the zero hash. If consensus is achieved
for the zero hash, than the block will be empty. 

One final difference between swell and tendermint is that within swell honest nodes are allowed to vote for a value for a hash 
even without in posession of the information that produces that hash. Also, a honest node can vote for the hash of an invalid 
hash, one that contains actions that are not correct. The objetive is to produce a consensus over the hash. The final block 
will be revalidated and expurious actiosn removed. While voting, each node will inform its peers if it has the data that produces
the hash for which the vote is cast. 

If a honest node receives _f + 1_ confirmations for the hash it can safely assume that the underlying data can be recovered from a 
honest node. Thus, in case it has those confirmations, the honest node will be allowed to commit to the value (according to the 
algorithm rules) even without posession of the data.

## Gossip Network and Messages

The committee leader build the block and broadcast it to all participants. No other node should try to build a block for a given epoch. 

Besides this block broadcast network, committee nodes are connected to each other for a consensus network. Within this network, any  new valid message received by any node is broadcast to all others except to that node that sent the new message. 

There are three kind of messages for this network:

| message               | Interpretation                                                                    |
|:---------------------:|-----------------------------------------------------------------------------------|
|⟨ _e<sub>s</sub>_ ⟩<sub>_r_</sub>    | proposal for the round _r_ of the value _e_ last validated on round _s_ or not validated (_s=-1_)     |
|⦅ _e<sub>yes</sub>_ ⦆<sub>_r_</sub> or ⦅ _e<sub>no</sub>_ ⦆<sub>_r_</sub>         | vote for the round _r_ to the value _e_ informing possession of underlying data.  |
| ⟦ _v_ ⟧<sub>_r_</sub>            | commit for the round _r_ to value _v_.                                            |


This multi-level, multi-round strategy ensures that all honest nodes will eventually agree on the value as long as less than _1/3_ are faulty or malicious and the communication channel is effective after a certain interval. 

## Algorithm

**Require:** a known leader selection function that defines the leader node for every round that is capable of proposing a value for the committee, and a valid hash value _h_ for the leader at the round zero. 

_Keep track of the round, validated values and the last round which was validated, commited values and the last round committed._ 

01 _r ← 0_ (starting at round zero)
\
02 _h<sub>v</sub> ← h_  and  _v ← 0_ (for the leader of the first round) _h<sub>s</sub> ← ∅_ and _s=-1_ (for all others)
\
03 _h<sub>c</sub> ←_ ∅  and  _c ← -1_ 

03 **upon** start **exectute** _NewRound_( _r_ )

04 **procedure** _NewRound_( _r'_ ):
\
05&nbsp;&nbsp;&nbsp;&nbsp; _r ← r'_
\
06&nbsp;&nbsp;&nbsp;&nbsp; **update** _state_ to _proposing_ 
\
07&nbsp;&nbsp;&nbsp;&nbsp; **if** node is leader for the current round **then:**
\
08&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp; **broadcast** proposal ⟨ _h<sub>c</sub>_ ⟩<sub>_r_</sub>
\
09&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp; **update** _status_ to _voting_
\
09&nbsp;&nbsp;&nbsp;&nbsp; **else:**
\
10&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp; **schedule** _TimeOutPropose_( _r_ ) to **execute after** _ΔT<sub>p</sub>_

_Honest node votes blank if proposal not received in time._

11 **procedure** _TimeoutPropose_( _r'_ ):
\
12&nbsp;&nbsp;&nbsp;&nbsp; **if** _r' = r_  **and** _state_ is _proposing_ **then:**
\
13&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp; **broadcast** blank vote ⦅ ∅  ⦆<sub>_r_</sub>
\
14&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp; **update** _status_ to _voting_

15 **procedure** _TimeoutVote_( _r'_ ):
\
16&nbsp;&nbsp;&nbsp;&nbsp; **if** _r' = r_  **and** _state_ is _voting_ **then:**
\
17&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp; **broadcast** blank commit  ⟦ ∅ ⟧<sub>_r_</sub>
\
18&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp; **update** _status_ to _committing_

_If there are no sufficient vote to commit move to the next round._

19 **procedure** _TimeoutCommit_( _r'_ ):
\
20&nbsp;&nbsp;&nbsp;&nbsp; **if** _r' = r_  **and** _state_ is _committing_ **then:**
\
17&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp; **execute** _NewRound_( _r + 1_ ) 

_The following statements are executed in any order based on new messages received by the node. Alterations on the global variables
based on any rule does not affect those same variables for the other rules. For example, if a rule changes the status variable, this
will not take effect until all the rules are processed. There are no ambiguous scenario on the rules, namelly there are no two rules
that can update a variable to incompatible values at the same pass._

18 **while** _state_ is _proposing_ **do**

_Honest node follows proposed value if not incompatible with a committed value. Otherwise vote blank._ 


19
&nbsp;&nbsp;&nbsp;&nbsp; 
**upon** proposal ⟨ _e<sub>s</sub>_ ⟩<sub>_r_</sub> of value _e_ for round _r_ last seen on round _s_ (or never seen s=-1) **do:**
\
20 &nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp; 
**if** not committed after _s_ or last committed to _e_ **then:**
\
21 &nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;
**broadcast** vote ⦅ _é_ ⦆<sub>_r_</sub> to value _e_ for the round _r_
\
22 &nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;
**else:**
\
23 &nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;
**broadcast** _blank_ vote for the round _r_
\
22 &nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;
**update** _status_ to _committing_

\
24 **while** _state_ is _voting_ **do**

25 &nbsp;&nbsp;&nbsp;&nbsp; 
**upon** _2×f + 1_ votes to any value for round _r_ **do once:** 
\
26 &nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp; 
**schedule** _TimeOutVote_( _r_ ) after _ΔT<sub>v</sub>_

_Given a proposed value and 2×f + 1 votes for that same value, granted knowledge of the underlying data for that produces a hash compatible with that value, trigger a commit:_

27 &nbsp;&nbsp;&nbsp;&nbsp;
**uppon** proposal ⟨ _e<sub>*</sub>_ ⟩<sub>_r_</sub> and _2×f + 1_ votes to _e_ for round _r_ **and** knowing _e_ **do:**
\
28 &nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;
**boardcast** commit ⟦ _e_ ⟧<sub>_r_</sub> to value _e_ for the round _r_
\
29 &nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;
**update** _state_  to _committing_ 

_Given 2×f + 1 blank votes trigger a blank commit:_

27 &nbsp;&nbsp;&nbsp;&nbsp;
**uppon** _2×f + 1_ blank votes for round _r_  **do:**
\
28 &nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;
**boardcast** blank commit ⟦ ∅ ⟧<sub>_r_</sub>
\
29 &nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;
**update** _state_  to _committing_ 


24 **while** _state_ is _voting_ **or** _committing_ **do:**

25 &nbsp;&nbsp;&nbsp;&nbsp; 
**upon** proposal ⟨ _h<sub>*</sub>_ ⟩<sub>_r_</sub> **and** _2×f + 1_ votes for _h_ on round _r_ **do:**
votes to any value for round _r_ **do once:** 
\
26 &nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp; 
_h<sub>v</sub> ← h **and** _v ← r_.

