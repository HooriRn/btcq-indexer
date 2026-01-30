'use strict';

g.v.updatePoolReturns = function (elem, poolReturns) {
    let p = poolReturns;
    // In Thornode/Midgard, to store certain values as integers without losing precision,
    // the values are scaled up by 1e8 before being truncated. Undo this scaling for
    // presentation purposes.
    for (let a of ['Rune', 'Asset']) {
        for (let metric of ['added', 'withdrawn', 'redeemable']) {
            p[metric + a] = p[metric + a] * 1e-8;
        }
    }
    for (let a of ['Usd', 'Rune', 'Asset']) {
        for (let metric of ['addedValueIn', 'withdrawnValueIn', 'redeemableValueIn',
                            'realizedReturnValueIn', 'totalReturnValueIn']) {
            p[metric + a] = p[metric + a] * 1e-8;
        }
    }
    elem.innerHTML = `
        <h3>Position Summary</h3>
        <table><tr>
                <th></th><th>RUNE</th><th>Asset</th>
            </tr><tr>
                <td>Added</td><td>${p.addedRune}</td><td>${p.addedAsset}</td>
            </tr><tr>
                <td>Withdrawn</td><td>${p.withdrawnRune}</td><td>${p.withdrawnAsset}</td>
            </tr><tr>
                <td>Redeemable</td><td>${p.redeemableRune}</td><td>${p.redeemableAsset}</td>
            </tr><tr>
                <td>Gain (Reedeemable + Withdrawn - Added)</td>
                <td>${(p.redeemableRune + p.withdrawnRune - p.addedRune)}</td>
                <td>${(p.redeemableAsset + p.withdrawnAsset - p.addedAsset)}</td>
            </tr><tr>
                <td>Gain % ((Redeemable + Withdrawn) / Added - 1)</td>
                <td>${((p.redeemableRune + p.withdrawnRune) / p.addedRune - 1) * 100}%</td>
                <td>${((p.redeemableAsset + p.withdrawnAsset) / p.addedAsset - 1) * 100}%</td>
            </tr>
        </table>
        <h3>Valuations</h3>
        <table><tr>
                <th></th><th>Method 1: USD</th><th>Method 2: RUNE</th><th>Method 3: Asset</th>
            </tr><tr>
                <td>Added value</td><td>${p.addedValueInUsd}</td><td>${p.addedValueInRune}</td><td>${p.addedValueInAsset}</td>
            </tr><tr>
                <td>Withdrawn value</td><td>${p.withdrawnValueInUsd}</td><td>${p.withdrawnValueInRune}</td><td>${p.withdrawnValueInAsset}</td>
            </tr><tr>
                <td>Redeemable value</td><td>${p.redeemableValueInUsd}</td><td>${p.redeemableValueInRune}</td><td>${p.redeemableValueInAsset}</td>
            </tr><tr>
                <td>Realized return value</td><td>${p.realizedReturnValueInUsd}</td><td>${p.realizedReturnValueInRune}</td><td>${p.realizedReturnValueInAsset}</td>
            </tr><tr>
                <td>Total return value</td><td>${p.totalReturnValueInUsd}</td><td>${p.totalReturnValueInRune}</td><td>${p.totalReturnValueInAsset}</td>
            </tr>
        </table>`;
}
