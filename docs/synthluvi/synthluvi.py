import csv
import math
import itertools
from multiprocessing import pool

def main():
    fname = 'tmp/export/all.csv'
    with open(fname, 'r') as f:
        r = csv.reader(f)
        lpUnits = 0

        asset = 0
        rune = 0
        synth = 0
        prevSynth = 0
        startLuvi = 0
        luvi = 0

        synthUnitsAfterChange = 0
        logLUVIIncreaseDueToSynths = 0

        valueChange = 0

        vChangePerUnit = 0

        for d in itertools.chain(r, [[0, 'lastdepth', 0, 0, 0]]):

            if d[1] == 'units':
                lpUnits = int(d[2])
                # print(lpUnits)
            if d[1] == 'depth':
                asset = int(d[2])
                rune = int(d[3])
                synth = int(d[4])

                synthUnits = lpUnits * synth / (2*asset - synth)
                poolUnits = lpUnits + synthUnits
                luvi = math.sqrt(rune*asset)/poolUnits

                # print(asset, rune, synth, luvi)

                if startLuvi == 0:
                    startLuvi = luvi

                if synth != prevSynth:
                    oldSynthNowUnits = lpUnits * prevSynth / (2*asset - prevSynth)
                    logLUVIIncreaseDueToSynths += math.log((lpUnits + synthUnitsAfterChange) / (lpUnits + oldSynthNowUnits))
                    synthUnitsAfterChange = synthUnits


            if d[1] == 'swap':
                # 2 = mint 3 = burn
                direction = int(d[2])
                fromE8 = int(d[3])
                toE8 = int(d[4])
                if direction == 2:
                    valueAfter = math.sqrt(asset*rune)
                    valueBefore = math.sqrt(asset*(rune-fromE8))
                    valueChange += valueAfter - valueBefore
                    vChangePerUnit += (valueAfter - valueBefore) / lpUnits

                if direction == 3:
                    valueAfter = math.sqrt(asset*rune)
                    valueBefore = math.sqrt(asset*(rune+toE8))
                    valueChange += valueAfter - valueBefore
                    vChangePerUnit += (valueAfter - valueBefore) / lpUnits

            if d[1] == 'lastdepth':
                # give out all synths value in asset
                valueAfter = math.sqrt((asset-synth)*rune)
                valueBefore = math.sqrt(asset*rune)
                valueChange += valueAfter - valueBefore
                vChangePerUnit += (valueAfter - valueBefore) / lpUnits

            prevSynth = synth

        print('LUVI increase', luvi/startLuvi)
        print('LUVIIncreaseDueToSynths: ', math.exp(logLUVIIncreaseDueToSynths))
        # print(valueChange / lpUnits / luvi)
        # print(valueChange / poolUnits / luvi)
        # print(valueChange / math.sqrt(asset*rune))
        print('ValueChangePerUnit/LUVI', 1 + vChangePerUnit / startLuvi)


if __name__ == "__main__":
    main()