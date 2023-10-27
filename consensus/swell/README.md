---
header-includes:
  - \usepackage[ruled,vlined,linesnumbered]{algorithm2e}
---

# Swell

Within swell _2×f + 1_ nodes exchange messages of different kind in order to form a consensus. 

| message               | Interpretation                                                                    |
|:---------------------:|-----------------------------------------------------------------------------------|
|⟨ _e<sub>s</sub>_ ⟩<sub>_r_</sub>    | proposal for the round _r_ of the value _e_ last validated on round _s_     |
|⟨ _e_  ⟩<sub>_r_</sub>          | proposal for the round _r_ of the value _v_ not validated                         |
|⦅ _é_ ⦆<sub>_r_</sub>         | vote for the round _r_ to the value _e_ informing possession of underlying data.  |
|⦅ _è_  ⦆<sub>_r_</sub>         | vote for the round _r_ to the value _e_ without possession of underlying data.    |
| ⟦ _v_ ⟧<sub>_r_</sub>            | commit for the round _r_ to value _v_.                                            |





_r<sub>val</sub>_

01 **while** _state_ is _proposing_ **do**

 
02
&nbsp;&nbsp;&nbsp;&nbsp; 
**upon** proposal ⟨ _e<sub>s</sub>_ ⟩<sub>_r_</sub> of value _e_ for round _r_ last seen on round _s_ (or never seen s=-1) **do:**
\
03 &nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp; 
**if** not committed after _s_ or last committed to _e_ **then:**
\
04&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;
**broadcast** vote ⦅ _é_ ⦆<sub>_r_</sub> to value _e_ for the round _r_
\
05&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;
**else:**
\
06&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;
**broadcast** _blank_ vote for the round _r_

\
07 **while** _state_ is _voting_ **do**

08&nbsp;&nbsp;&nbsp;&nbsp; 
**upon** _2×f + 1_ votes to any value for round _r_ **do once:** 
\
09 &nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp; 
**schedule** _TimeOutVote_( _r_ ) after _ΔT<sub>v</sub>_

10&nbsp;&nbsp;&nbsp;&nbsp;
**uppon** proposal ⟨ _e<sub>*</sub>_ ⟩<sub>_r_</sub> and _2×f + 1_ votes to _e_ for round _r_ **and** knowing _e_ **do:**
\
11&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;
**boardcast** commit ⟦ _e_ ⟧<sub>_r_</sub> to value _e_ for the round _r_
\
12&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;
**update** _state_  to _committing_ 
