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

**while** _state_ is _proposing_ **do**

&nbsp;&nbsp;&nbsp;&nbsp; 
**receiving** proposal for round _r_ of value _v_  never confirmed:
\
&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp; 
**if** not committed or last committed to _e_ **then**
\
&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;
**broadcasta* _blank_ vote for the round _r_
\
&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;
**else**
\
&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;
**broadcast** 

&nbsp;&nbsp;&nbsp;&nbsp; 
**receiving** proposal ⟨ _p_ · _s_ ⟩<sub>_r_</sub> and _2f + 1_ votes to _p_ for round _r<sub>p</sub>_ **do**
\
&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;
**if** mot committed ater _r<sub>p

for value _p_ seen on round _0_ **do**:
\
&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp; 
**if** not committed pr last committed to _p_ **then**
\
&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;
**broadcast** vote to ⟨_p_ | 0⟩<sub>r</sub> 
\
&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;
**else**
\
&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;
**broadcast** 
