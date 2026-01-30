# Earnings explained

Defining and calculating gains (earnings, returns on investment) of a liquidity provider in a pool
is a surprisingly non-trivial task. In this document we explore different approaches and explain
our definitions.

## Definitions

### Partial pool ownership

A member's share of a pool is defined in terms of "pool units":

```
A  = total amount of Asset in the pool
R  = total amount of Rune in the pool
Um = pool units of a specific member in the pool
U  = total sum of pool units in the pool
```

With this setup a given member's share of the pool is `R * Um / U` Rune and `A * Um / U` Asset.
These are the amounts the member would get if they withdrew all of their liquidity at this point.

### Price

The definition of price follows from the fundamental assumption that the value of all Rune and
all Asset in the pool is always the same. Thus, the price of Asset in Rune is:

`p = R / A`

This means that `p` Rune can be swapped for `1` Asset.

### Pool ownership is always symmetrical

We call a deposit symmetric when the member provides both Rune and Asset to the pool in such a ratio
that it doesn't move the price (that is, the deposit's Rune to Asset ratio is the same as the
current Rune to Asset ratio in the pool.)

For convenience, members can deposit or withdraw asymmetrically, which means that they use
a different ratio, often just providing one side (only Rune or only Asset). But note that after
the deposit has happened, pool ownership is represented as a single number, an amount pool units
`Um`, effectively meaning that the member's deposit is converted to an evenly balanced combination
of Rune and Asset (half-and-half by value) right away. So, even though deposits (and withdraws)
can be in any mixture of Rune and Asset, a share in a pool always represents a balanced combination
of them.

## Changing value; Pooled vs HOLD

In this section we lay the groundwork for calculating the return on investment of providing
liquidity in a pool. For this we compare how the "value" of pool membership changes over time
_compared_ to how the value of original funds would have developed if held outside of the pool.

Over time, the amount of pool units of the member, `Um`, stays constant (we are analyzing here a
single round from depositing liquidity to fully withdrawing it), but the corresponding amount of
Rune and Asset in the pool changes, due to:

* Fees collected in the pool increase the Rune and Asset amount per pool unit
* As the price shifts (by users doing swaps etc.), the ratio of Rune and Asset shift correspondingly

Let's look at an example where a member deposits then later withdraws, both symmetrically:

```
Original price: 10 (1 Asset = 10 Rune)
Deposit: 100 Asset, 1000 Rune

How much was this deposit worth at the time?
Deposit value in Rune: 2000 Rune
Deposit value in Asset: 200 Asset

Time passes, and overall:

1. price shifts to 2.5 (from the initial 10); without fees the share of the new pool would be:
   200 Asset, 500 Rune
2. some fees are collected, say 10%: this means additional 20 Asset, 50 Rune

Withdraw all liquidity: 220 Asset, 550 Rune
How much is this worth at that time?
Withdrawn value in Rune: 1100 Rune
Withdrawn value in Asset: 440 Asset
```

(Exercise: why is the resulting pool share in `1.` the claimed 200 Asset, 500 Rune? Remember,
the basic invariant is that without fees `A*R` stays constant.)

Thus we can see that how much one have gained or lost depends very much on how one thinks of
what would the HOLD position mean. Some obvious options:

1. Holding the original amount in Rune vs withdrawn value (2000 -> 1100 = -45%)
2. Holding the original amount in Asset vs withdrawn value (200 -> 440 = +120%)
3. Holding the original position, half in Rune and half in Asset vs withdrawn value:
   Original position's current value (in Rune) = 100*2.5 + 1000 = 1250. Gain: 1250 -> 1100 = -12%

In this document we will focus on option 3. as a baseline, because:
* Options 1. and 2. are volatile, mostly a reflection of price and not the fees collected.
  One can be typically negative while the other is positive.
* ThorNode considers pool ownership as half rune half asset.
* Option 3. is also sensitive to both fees collected and impermanent loss. In the example above
  it was negative due to the large price shift and relatively little collected fees.
* ThorNode uses 3. for calculating impermanent loss protection. That means, that if the gain
  calculated according to 3. is negative at the time of a withdraw, then a corresponding amount
  is paid out to the member to compensate for this loss.

So in this document we will define gain as

```
GainRatio = (WithdrawnAsset * newPrice + WithdrawnRune) / (DepositedAsset * newPrice + DepositedRune)
```

Which simplifies to

```
GainRatio = 2 * WithdrawnRune / (DepositedAsset * newPrice + DepositedRune)
```

But note, that using our tools it is also easy to calculate gain/loss corresponding
to options 1. and 2.

# Gain/loss for some time interval

For a given deposit date and withdraw date of a hypothetical member then we can calculate
gain/loss ratio, without needing to know their actual deposit amounts, just by following how much
a single pool unit is worth at any given time.

Value of a single pool unit: `A/U` Asset, `R/U` Rune

## GainRatio decomposition

We are going to decompose the GainRatio into two factors:
* LUVI, which is price-independent and represent how much the pool has grown due to collected fees
* PriceShiftLoss, the impermanent loss due the price shift.

Definitions:
```
A0 = total amount of Asset in the pool at the beginning of the time interval
R0 = total amount of Rune in the pool at the beginning of the time interval
U0 = total Pool units of pool at the beginning of the time interval

A1 = total amount of Asset in the pool at the end of the time interval
R1 = total amount of Rune in the pool at the end of the time interval
U1 = total Pool units of pool at the end of the time interval
```

## Liquidity Unit Value Index (LUVI)

ThorChain's principal formula for a pool is `A*R=K`. Ideal swaps (without fees) would keep the pool
on this curve, but fees, rewards and donations increase `K`.

Definition:
```
LUVI = sqrt(A*R) / U
```

With this we can define the increase over a time interval:
```
LUVI0 = sqrt(A0*R0) / U0
LUVI1 = sqrt(A1*R1) / U1

LUVI_Increase = LUVI1 / LUVI0
```

Note that `LUVI_Increase`:
* is exactly the GainRatio if there is no price change (for all of the gain definitions)
* always at least `1`

## Price Shift Loss

If we assume perfect swaps keeping the `K` constant in `A*R=K` then a change in price will cause an
"impermanent loss".

```
Price0 = R0 / A0
Price1 = R1 / A1
PriceShift = Price1 / Price0
PriceShiftLoss = 2*sqrt(PriceShift) / (1 + PriceShift)
```

Note that `PriceShiftLoss` is always less then `1`, independently of the direction
of the price shift. Explore its characteristics here:

https://www.desmos.com/calculator/u7oa6z6wum

Unlike `LUVI`, `PriceShiftLoss` doesn't simply accumulate over consecutive time intervals, e.g.
two impermanent losses of 0.95 in a row may end up cancelling each other if they correspond to
price shifts in opposite directions.

We note that similar formulas for `PriceShiftLoss` can be developed for gain/loss calculation based
on other models.
For example, for pure Rune-based calculation (option #1 above) `PriceShiftLoss = sqrt(PriceShift)`,
and for pure Asset-based calculation: `PriceShiftLoss = 1/sqrt(PriceShift)`. Note that here it's
no longer the case that `PriceShiftLoss` is never above `1`, a price shift in an appropriate
direction might mean an "impermanent gain".

## GainRatio = LUVI * PriceShiftLoss

As previously mentioned, the definition of GainRatio is:

```
GainRatio = 2 * WithdrawnRune / (DepositedAsset * newPrice + DepositedRune)
```

With a simple calculation we can verify that this decomposes into:

```
GainRatio = LUVIIncrease * PriceShiftLoss
```

In the above example, arbitrarily assigning pool units, say `U = 100`

```
GainRatio = 2 * WithdrawRune / (DepositAsset * newPrice + DepositRune)
          =  1100 / (100*2.5 + 1000)
          = 0.88

LUVI0 = [sqrt(100*1000) / 100] = 3.16227
LUVI1 = [sqrt(220*550) / 100] = 3.47850
LUVI_Increase = LUVI1 / LUVI0 = 1.1

Price0 = 10
Price1 = 2.5
PriceShift = 2.5 / 10 = 0.25
PriceShiftLoss = 2*sqrt(PriceShift) / (1 + PriceShift)
               = 0.8

GainRatio = LUVIIncrease * PriceShiftLoss
          = 1.1 * 0.8
          = 0.88
```

Due to the large price shift, this example has a 20% impermanent loss and a 10% gain via
collection of fees, which cumulatively results in a 12% overall loss.
