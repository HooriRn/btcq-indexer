'use strict';

class LPLiquidity {
    // Return on investment as measured in USD, RUNE or the pool asset.

    constructor() {
    }
    update(memberDetails, poolDetails, actions, assetPriceInRuneByTime, assetPriceInUsdByTime) {
        this.pool = memberDetails.pool;
        this.memberDetails = memberDetails;
        this.poolDetails = poolDetails;
        this.updateAddWithdrawnValueInRune(actions, assetPriceInRuneByTime, assetPriceInUsdByTime);

        // TODO(leifthelucky): Check how to divide this correctly without losing precision.
        this.redeemableRune = memberDetails.liquidityUnits / poolDetails.liquidityUnits * poolDetails.runeDepth;
        this.redeemableAsset = memberDetails.liquidityUnits / poolDetails.liquidityUnits * poolDetails.assetDepth;

        this.realizedReturnValueInRune = this.withdrawnValueInRune - this.addedValueInRune;
        this.realizedReturnValueInAsset = this.withdrawnValueInAsset - this.addedValueInAsset;
        this.realizedReturnValueInUsd = this.withdrawnValueInUsd - this.addedValueInUsd;

        this.redeemableValueInRune = this.redeemableRune + this.redeemableAsset * poolDetails.assetPrice;
        this.redeemableValueInAsset = this.redeemableRune / poolDetails.assetPrice + this.redeemableAsset;
        this.redeemableValueInUsd = this.redeemableRune / poolDetails.assetPrice * poolDetails.assetPriceUSD + this.redeemableAsset * poolDetails.assetPriceUSD;

        this.totalReturnValueInRune = this.realizedReturnValueInRune + this.redeemableValueInRune;
        this.totalReturnValueInAsset = this.realizedReturnValueInAsset + this.redeemableValueInAsset;
        this.totalReturnValueInUsd = this.realizedReturnValueInUsd + this.redeemableValueInUsd;
    }
    updateAddWithdrawnValueInRune(actions, assetPriceInRuneByTime, assetPriceInUsdByTime) {
        this.addedRune = 0;
        this.addedAsset = 0;
        this.addedValueInRune = 0;
        this.addedValueInAsset = 0;
        this.addedValueInUsd = 0;
        this.withdrawnRune = 0;
        this.withdrawnAsset = 0;
        this.withdrawnValueInRune = 0;
        this.withdrawnValueInAsset = 0;
        this.withdrawnValueInUsd = 0;
        for (const action of actions) {
            if (action.status != "success") {
                continue;
            }
            if (action.pools.length != 1 || action.pools[0] != this.pool) {
                continue;
            }
            const valueInRune = function (coin, assetFilter, assetPriceRune) {
                switch (coin.asset) {
                    case "THOR.RUNE":
                        return Number(coin.amount);
                    case assetFilter:
                        return Number(coin.amount) * assetPriceRune;
                    default:
                        return;
                }
            }
            const valueInAsset = function (coin, assetFilter, assetPriceRune) {
                switch (coin.asset) {
                    case "THOR.RUNE":
                        return Number(coin.amount) / assetPriceRune;
                    case assetFilter:
                        return Number(coin.amount);
                    default:
                        return;
                }
            }
            const valueInUsd = function (coin, assetFilter, assetPriceRune, assetPriceUsd) {
                switch (coin.asset) {
                    case "THOR.RUNE":
                        return Number(coin.amount) / assetPriceRune * assetPriceUsd;
                    case assetFilter:
                        return Number(coin.amount) * assetPriceUsd;
                    default:
                        return;
                }
            }
            const s = Math.floor(action.date / 1e9);
            const assetPriceRune = assetPriceInRuneByTime[s];
            const assetPriceUsd = assetPriceInUsdByTime[s];
            let inRune = 0;
            let inAsset = 0;
            let inValueInRune = 0;
            let inValueInAsset = 0;
            let inValueInUsd = 0;
            let outRune = 0;
            let outAsset = 0;
            let outValueInRune = 0;
            let outValueInAsset = 0;
            let outValueInUsd = 0;
            for (const inputs of action.in) {
                for (const coin of inputs.coins) {
                    inRune += (coin.asset == "THOR.RUNE") ? Number(coin.amount) : 0;
                    inAsset += (coin.asset == this.pool) ? Number(coin.amount) : 0;
                    inValueInRune += valueInRune(coin, this.pool, assetPriceRune);
                    inValueInAsset += valueInAsset(coin, this.pool, assetPriceRune);
                    inValueInUsd += valueInUsd(coin, this.pool, assetPriceRune, assetPriceUsd);
                }
            }
            for (const inputs of action.out) {
                for (const coin of inputs.coins) {
                    outRune += (coin.asset == "THOR.RUNE") ? Number(coin.amount) : 0;
                    outAsset += (coin.asset == this.pool) ? Number(coin.amount) : 0;
                    outValueInRune += valueInRune(coin, this.pool, assetPriceRune);
                    outValueInAsset += valueInAsset(coin, this.pool, assetPriceRune);
                    outValueInUsd += valueInUsd(coin, this.pool, assetPriceRune, assetPriceUsd);
                }
            }
            switch (action.type) {
                case "withdraw":
                    // In rune/asset/value contain the amount in the withdraw
                    // transaction, that is the one with the withdraw memo.
                    this.withdrawnRune += outRune - inRune;
                    this.withdrawnAsset += outAsset - inAsset;
                    this.withdrawnValueInRune += outValueInRune - inValueInRune;
                    this.withdrawnValueInAsset += outValueInAsset - inValueInAsset;
                    this.withdrawnValueInUsd += outValueInUsd - inValueInUsd;
                    break;
                case "addLiquidity":
                    // Out rune/asset/value should be 0 for addLiquidity events,
                    // but we include them here just in case.
                    this.addedRune += inRune - outRune;
                    this.addedAsset += inAsset - outAsset;
                    this.addedValueInRune += inValueInRune - outValueInRune;
                    this.addedValueInAsset += inValueInAsset - outValueInAsset;
                    this.addedValueInUsd += inValueInUsd - outValueInUsd;
                    break;
            }
        }
    }
};

g.m.LPLiquidity = new LPLiquidity();

