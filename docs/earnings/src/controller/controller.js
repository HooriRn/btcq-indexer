'use strict';

// main namespace g = "global"
var g = { m: {}, v: {}, c: {} };

g.c.queryEarnings = function () {
    const form = document.forms["lp"];

    const midgardUrl = "https://midgard.ninerealms.com/v2";
    const poolInput = form.pool.value;
    const addressInput = form.address.value;

    // TODO(leifthelucky): For consistency, the pool and member details data
    // should be requested as of a certain block height or timestamp. This
    // ensures not only correct numbers, but reproducibility, critical for
    // debugging. Unfortunately, the Midgard /member and /pool endpoints do
    // not accept a "block" parameter.
    let memberDetailsUrl = `${midgardUrl}/member/${addressInput}`;
    let poolUrl = `${midgardUrl}/pool/${poolInput}`;
    Promise.all([
        fetch(new Request(memberDetailsUrl)),
        fetch(new Request(poolUrl)),
    ]).then(function (responses) {
        return Promise.all(responses.map(function (res) {
            if (!res.ok) {
                throw new Error(`HTTP error! status: ${res.status}`);
            }
            return res.json();
        }));
    }).then(function (data) {
        if (data.length !== 2) {
            throw new Error(`Wrong number of responses.`);
        }
        const memberDetailsData = data[0]
        const poolData = data[1]
        for (const memberPoolData of memberDetailsData.pools) {
            if (memberPoolData.pool == poolInput) {
                const addrRune = addressInput.startsWith('thor') ? addressInput : memberPoolData.runeAddress;
                const addrAsset = !addressInput.startsWith('thor') ? addressInput : memberPoolData.assetAddress;

                let actions = []
                let actionsUrl = `${midgardUrl}/actions?address=${addrRune},${addrAsset}` +
                    `&type=addLiquidity,withdraw`;
                const firstActionPromise = fetch(new Request(actionsUrl)).then(resp => resp.json())
                firstActionPromise.then(function (actionsFirstPage) {
                    actions = actionsFirstPage.actions;
                    let moreActionsPromises = [];
                    const limit = 50;
                    const pageCount = Math.floor((actionsFirstPage.count -1) / limit) + 1;
                    for(let i=1; i<pageCount; i++) { //start with 1, as we already have the first one above
                        let actionsUrlPaged = actionsUrl + `&limit=` + limit+ `&offset=` + (i * limit);
                        let actionPromise = fetch(new Request(actionsUrlPaged)).then(resp => resp.json())
                        moreActionsPromises.push(actionPromise);
                    }
                    Promise.all(moreActionsPromises).then(values => {
                        values.forEach(function (actionsDataPage) {
                            actions.push(...actionsDataPage.actions);
                        })
                    }).then(function () {
                        let historyPromises = []
                        // Fetch price data for all blocks appearing in the actions data.
                        const historyBaseUrl = `${midgardUrl}/history/depths/${memberPoolData.pool}`
                        actions.forEach(function (a) {
                            if (!a.pools.find(p => p == poolInput)) {
                                return;
                            }
                            const dNano = a.date;
                            const dSec = Math.floor(dNano / 1e9);
                            const url = historyBaseUrl + `?from=${dSec}&to=${dSec}`;
                            historyPromises.push(fetch(new Request(url)));
                        });
                        Promise.all(historyPromises).then(function (responses) {
                            return Promise.all(responses.map(function (res) {
                                if (!res.ok) {
                                    throw new Error(`HTTP error! status: ${res.status}`);
                                }
                                return res.json();
                            }));
                        }).then(function (historyData) {
                            let assetPriceInRuneByTime = {}
                            let assetPriceInUsdByTime = {}
                            historyData.map(function (item) {
                                assetPriceInRuneByTime[item.meta.startTime] = item.intervals[0].assetPrice;
                                assetPriceInUsdByTime[item.meta.startTime] = item.intervals[0].assetPriceUSD;
                            });
                            g.m.LPLiquidity.update(memberPoolData, poolData, actions,
                                assetPriceInRuneByTime, assetPriceInUsdByTime);
                            document.getElementById("view").innerHTML =
                                `<h3>Raw data</h3><pre>${JSON.stringify(g.m.LPLiquidity, null, '\t')}</pre>`
                            g.v.updatePoolReturns(document.getElementById("poolReturns"), g.m.LPLiquidity)
                        });
                    });
                });
                break;
            }
        }
    }).catch(function (error) {
        console.log(error);
    });
}
